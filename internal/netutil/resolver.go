package netutil

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var defaultPublicDNSServers = []string{
	"223.5.5.5:53",
	"119.29.29.29:53",
	"1.1.1.1:53",
	"8.8.8.8:53",
}

// rawOSResolver 始终调用底层操作系统原始解析器（不读取被替换后的 net.DefaultResolver）
var rawOSResolver = &net.Resolver{PreferGo: false}

// Default 全局安全直连解析与拨号器
var Default = &DirectResolver{
	osLookup:    rawOSResolver.LookupIPAddr,
	adapterDNS:  systemAdapterDNSServers,
	fallbackDNS: defaultPublicDNSServers,
}

var processResolverOnce sync.Once

// DirectResolver 在检测到 Winsock 合成回环 Fake-IP（如 Proxifier 127.107.x.x）时自动切换至纯 Go UDP DNS 解析
type DirectResolver struct {
	osLookup    func(ctx context.Context, host string) ([]net.IPAddr, error)
	adapterDNS  func() []string
	fallbackDNS []string
	verifiedDNS atomic.Value // string
}

// NormalizeServerEndpoint 规范化控制面 HTTP(S) 地址为带显式默认端口的 host:port 并返回是否启用 TLS
func NormalizeServerEndpoint(raw string) (hostPort string, useTLS bool) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimRight(trimmed, "/")
	defaultPort := "80"

	switch {
	case strings.HasPrefix(strings.ToLower(trimmed), "https://"):
		useTLS = true
		defaultPort = "443"
		trimmed = trimmed[len("https://"):]
	case strings.HasPrefix(strings.ToLower(trimmed), "http://"):
		useTLS = false
		defaultPort = "80"
		trimmed = trimmed[len("http://"):]
	}

	if idx := strings.IndexByte(trimmed, '/'); idx >= 0 {
		trimmed = trimmed[:idx]
	}

	if _, _, err := net.SplitHostPort(trimmed); err == nil {
		return trimmed, useTLS
	}
	trimmed = strings.TrimPrefix(strings.TrimSuffix(trimmed, "]"), "[")
	return net.JoinHostPort(trimmed, defaultPort), useTLS
}

// ExtractHost 从 URL 或 host:port 中提取纯主机名
func ExtractHost(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "://") {
		if u, err := url.Parse(trimmed); err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
	}
	hostPort, _ := NormalizeServerEndpoint(trimmed)
	if h, _, err := net.SplitHostPort(hostPort); err == nil {
		return h
	}
	return trimmed
}

// IsSyntheticLoopbackFakeIP 判断外部域名是否被 Winsock Hook（如 Proxifier）劫持为非 127.0.0.1 的 127.x.y.z 合成回环地址
func IsSyntheticLoopbackFakeIP(host string, ips []net.IPAddr) bool {
	cleanHost := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if cleanHost == "" || net.ParseIP(cleanHost) != nil {
		return false
	}
	if cleanHost == "localhost" ||
		strings.HasSuffix(cleanHost, ".localhost") ||
		strings.HasSuffix(cleanHost, ".local") ||
		HasManagedDomainSuffix(cleanHost) {
		return false
	}
	if len(ips) == 0 {
		return false
	}

	loopbackv4 := net.IPv4(127, 0, 0, 1)
	for _, item := range ips {
		ip4 := item.IP.To4()
		if ip4 == nil {
			// 含有真实 IPv6 记录时不视为纯 127.x 合成回环
			return false
		}
		if ip4[0] != 127 || ip4.Equal(loopbackv4) {
			return false
		}
	}
	return true
}

// VerifiedDNSServer 返回最近一次验证可用的上游 UDP DNS 服务器地址
func (r *DirectResolver) VerifiedDNSServer() string {
	if v := r.verifiedDNS.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ResolveIPAddr 解析域名；若命中 Winsock 合成回环地址则自动绕过 GetAddrInfoW 查询上游 UDP DNS
func (r *DirectResolver) ResolveIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IPAddr{{IP: ip}}, nil
	}

	lookupFn := rawOSResolver.LookupIPAddr
	if r.osLookup != nil {
		lookupFn = r.osLookup
	}

	ips, osErr := lookupFn(ctx, host)
	if osErr == nil && !IsSyntheticLoopbackFakeIP(host, ips) {
		return ips, nil
	}

	candidates := r.collectCandidateDNSServers()
	for _, ns := range candidates {
		udpResolver := &net.Resolver{
			PreferGo: true,
			Dial: func(dialCtx context.Context, _, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: 1500 * time.Millisecond}
				return d.DialContext(dialCtx, "udp", ns)
			},
		}
		subCtx, cancel := context.WithTimeout(ctx, 1800*time.Millisecond)
		udpIPs, err := udpResolver.LookupIPAddr(subCtx, host)
		cancel()
		if err != nil || len(udpIPs) == 0 {
			continue
		}
		filtered := filterOutSyntheticLoopback(host, udpIPs)
		if len(filtered) > 0 {
			r.verifiedDNS.Store(ns)
			log.Printf("[NetUtil] 检测到 %s 被系统解析为合成回环地址，已通过 UDP DNS (%s) 解析真实地址", host, ns)
			return filtered, nil
		}
	}

	if osErr != nil {
		return nil, osErr
	}
	return ips, nil
}

// DialContext 先执行防合成回环解析，再按顺序直连目标 TCP 端口
func (r *DirectResolver) DialContext(ctx context.Context, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid dial address %q: %w", addr, err)
	}

	ips, err := r.ResolveIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP addresses resolved for %s", host)
	}

	var lastErr error
	dialer := &net.Dialer{}
	for _, ipAddr := range ips {
		conn, dialErr := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ipAddr.IP.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	return nil, lastErr
}

func (r *DirectResolver) collectCandidateDNSServers() []string {
	seen := make(map[string]struct{})
	var result []string
	add := func(ns string) {
		ns = strings.TrimSpace(ns)
		if ns == "" {
			return
		}
		if _, _, err := net.SplitHostPort(ns); err != nil {
			ns = net.JoinHostPort(ns, "53")
		}
		if _, exists := seen[ns]; !exists {
			seen[ns] = struct{}{}
			result = append(result, ns)
		}
	}

	if prev := r.VerifiedDNSServer(); prev != "" {
		add(prev)
	}
	if r.adapterDNS != nil {
		for _, ns := range r.adapterDNS() {
			add(ns)
		}
	}
	for _, ns := range r.fallbackDNS {
		add(ns)
	}
	return result
}

func filterOutSyntheticLoopback(host string, ips []net.IPAddr) []net.IPAddr {
	if !IsSyntheticLoopbackFakeIP(host, ips) {
		var clean []net.IPAddr
		loopbackv4 := net.IPv4(127, 0, 0, 1)
		for _, item := range ips {
			if ip4 := item.IP.To4(); ip4 != nil && ip4[0] == 127 && !ip4.Equal(loopbackv4) {
				continue
			}
			clean = append(clean, item)
		}
		if len(clean) > 0 {
			return clean
		}
		return ips
	}
	return nil
}

// DialContext 全局安全 TCP 拨号入口（供 gRPC 与 HTTP Transport 共用）
func DialContext(ctx context.Context, addr string) (net.Conn, error) {
	return Default.DialContext(ctx, addr)
}

// EnsureProcessSafeResolverForTarget 对目标地址做一次合成回环探测；若当前进程受 Winsock Hook 影响，
// 则原地升级 *net.DefaultResolver 与 http.DefaultTransport，确保 tsnet（Headscale / DERP）与升级检查同步免疫。
func EnsureProcessSafeResolverForTarget(rawTarget string) {
	host := ExtractHost(rawTarget)
	if host == "" || net.ParseIP(host) != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	osIPs, err := rawOSResolver.LookupIPAddr(ctx, host)
	if err != nil || !IsSyntheticLoopbackFakeIP(host, osIPs) {
		return
	}

	if _, resolveErr := Default.ResolveIPAddr(ctx, host); resolveErr != nil {
		return
	}

	processResolverOnce.Do(func() {
		*net.DefaultResolver = net.Resolver{
			PreferGo: true,
			Dial: func(dialCtx context.Context, _, _ string) (net.Conn, error) {
				ns := Default.VerifiedDNSServer()
				if ns == "" {
					candidates := Default.collectCandidateDNSServers()
					if len(candidates) > 0 {
						ns = candidates[0]
					} else {
						ns = "223.5.5.5:53"
					}
				}
				d := net.Dialer{Timeout: 2 * time.Second}
				return d.DialContext(dialCtx, "udp", ns)
			},
		}
		if tr, ok := http.DefaultTransport.(*http.Transport); ok {
			tr.DialContext = func(dialCtx context.Context, _, addr string) (net.Conn, error) {
				return Default.DialContext(dialCtx, addr)
			}
		}
		log.Printf("[NetUtil] 已为进程启用纯 Go UDP DNS 解析器（上游: %s），tsnet 与 HTTP 客户端同步绕过合成回环地址", Default.VerifiedDNSServer())
	})
}
