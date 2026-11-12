package netutil

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
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

var defaultDoHEndpoints = []string{
	"https://223.5.5.5/dns-query",
	"https://1.12.12.12/dns-query",
}

// rawOSResolver 始终调用底层操作系统原始解析器（不读取被替换后的 net.DefaultResolver）
var rawOSResolver = &net.Resolver{PreferGo: false}

// Default 全局安全直连解析与拨号器
var Default = &DirectResolver{
	osLookup:     rawOSResolver.LookupIPAddr,
	adapterDNS:   systemAdapterDNSServers,
	fallbackDNS:  defaultPublicDNSServers,
	dohEndpoints: defaultDoHEndpoints,
}

var (
	processResolverOnce sync.Once
	localCleanDNSAddr   string
)

// DirectResolver 在检测到代理 Fake-IP（Proxifier 127.x 或 OpenClash/Mihomo 198.18.0.0/15）时自动切换至干净 UDP DNS / DoH 解析
type DirectResolver struct {
	osLookup     func(ctx context.Context, host string) ([]net.IPAddr, error)
	adapterDNS   func() []string
	fallbackDNS  []string
	dohEndpoints []string
	verifiedDNS  atomic.Value // string
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

func isFakeIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	// 1. Proxifier / Winsock 合成回环地址：非 127.0.0.1 的 127.0.0.0/8
	if ip4[0] == 127 && !ip4.Equal(net.IPv4(127, 0, 0, 1)) {
		return true
	}
	// 2. OpenClash / Clash / Mihomo / Sing-box 标准 Fake-IP 网段：198.18.0.0/15 (RFC 2544)
	if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
		return true
	}
	return false
}

func isClashTunFakeIPv4(ip net.IP) bool {
	ip4 := ip.To4()
	return ip4 != nil && ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19)
}

// IsSyntheticLoopbackFakeIP 判断外部域名是否被透明代理或 Winsock Hook 劫持为 Fake-IP（含 127.x 合成回环与 198.18.0.0/15 OpenClash Fake-IP）
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

	for _, item := range ips {
		ip4 := item.IP.To4()
		if ip4 == nil {
			return false
		}
		if !isFakeIPv4(ip4) {
			return false
		}
	}
	return true
}

// VerifiedDNSServer 返回最近一次验证可用的上游干净 DNS 标识
func (r *DirectResolver) VerifiedDNSServer() string {
	if v := r.verifiedDNS.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ResolveIPAddr 解析域名；若命中代理 Fake-IP（127.x / 198.18.0.0/15）则自动通过干净 UDP DNS 或 DoH 获取真实公网地址
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

	// 1. 尝试直接向候选 UDP DNS 查询（若路由器 DNS 返回 198.18.x.x 会被自动过滤并继续尝试公网 DNS）
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
		filtered := filterOutFakeIPs(host, udpIPs)
		if len(filtered) > 0 {
			r.verifiedDNS.Store(ns)
			log.Printf("[NetUtil] 检测到 %s 被代理劫持为 Fake-IP，已通过 UDP DNS (%s) 解析真实地址", host, ns)
			return filtered, nil
		}
	}

	// 2. 若透明网关把所有 53/UDP 查询也重定向劫持为 198.18.x.x，则使用纯 IP 直连 DoH (RFC 8484) 穿透获取真实 A 记录
	for _, dohURL := range r.dohEndpoints {
		dohIPs, err := queryDoHARecords(ctx, dohURL, host)
		if err != nil || len(dohIPs) == 0 {
			continue
		}
		filtered := filterOutFakeIPs(host, dohIPs)
		if len(filtered) > 0 {
			r.verifiedDNS.Store(dohURL)
			log.Printf("[NetUtil] 检测到 %s 的 UDP 53 亦被网关 Fake-IP 劫持，已通过 DoH (%s) 解析真实地址", host, dohURL)
			return filtered, nil
		}
	}

	if osErr != nil {
		return nil, osErr
	}
	return ips, nil
}

// DialContext 先执行防 Fake-IP 解析并优先直连真实目标 TCP 端口；若处于严格仅 TUN 出口环境则兜底回退原 198.18.x.x 地址
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
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	for _, ipAddr := range ips {
		conn, dialErr := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ipAddr.IP.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}

	// 若真实公网地址在强隔离内网无法直接 TCP 连通，且系统原始解析返回的是 198.18.0.0/15 TUN 地址，则兜底尝试 TUN 通道
	lookupFn := rawOSResolver.LookupIPAddr
	if r.osLookup != nil {
		lookupFn = r.osLookup
	}
	if osIPs, osErr := lookupFn(ctx, host); osErr == nil {
		for _, osIP := range osIPs {
			if isClashTunFakeIPv4(osIP.IP) {
				if conn, tunErr := dialer.DialContext(ctx, "tcp", net.JoinHostPort(osIP.IP.String(), port)); tunErr == nil {
					return conn, nil
				}
			}
		}
	}

	return nil, lastErr
}

func (r *DirectResolver) collectCandidateDNSServers() []string {
	seen := make(map[string]struct{})
	var result []string
	add := func(ns string) {
		ns = strings.TrimSpace(ns)
		if ns == "" || strings.HasPrefix(ns, "https://") {
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

func filterOutFakeIPs(host string, ips []net.IPAddr) []net.IPAddr {
	if HasManagedDomainSuffix(host) {
		return ips
	}
	var clean []net.IPAddr
	for _, item := range ips {
		if ip4 := item.IP.To4(); ip4 != nil && isFakeIPv4(ip4) {
			continue
		}
		clean = append(clean, item)
	}
	return clean
}

// queryDoHARecords 通过纯 IP 直连 HTTPS (RFC 8484 wireformat) 查询域名真实 A 记录，彻底免疫路由器 53/UDP 透明劫持
func queryDoHARecords(ctx context.Context, dohURL string, host string) ([]net.IPAddr, error) {
	wireQuery, err := buildSimpleARecordQuery(host)
	if err != nil {
		return nil, err
	}

	subCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()

	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: true,
		DialContext:       (&net.Dialer{Timeout: 1800 * time.Millisecond}).DialContext,
	}
	defer tr.CloseIdleConnections()

	req, err := http.NewRequestWithContext(subCtx, http.MethodPost, dohURL, bytes.NewReader(wireQuery))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	resp, err := (&http.Client{Transport: tr, Timeout: 2500 * time.Millisecond}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("doh status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return nil, err
	}
	return parseDNSARecordAnswers(body)
}

func buildSimpleARecordQuery(host string) ([]byte, error) {
	clean := strings.TrimSuffix(strings.TrimSpace(host), ".")
	if clean == "" {
		return nil, fmt.Errorf("empty host")
	}
	pkt := make([]byte, 12)
	binary.BigEndian.PutUint16(pkt[0:2], 0x1357)
	binary.BigEndian.PutUint16(pkt[2:4], 0x0100) // RD=1
	binary.BigEndian.PutUint16(pkt[4:6], 1)      // QDCOUNT=1
	for _, label := range strings.Split(clean, ".") {
		if len(label) == 0 || len(label) > 63 {
			return nil, fmt.Errorf("invalid dns label %q", label)
		}
		pkt = append(pkt, byte(len(label)))
		pkt = append(pkt, []byte(label)...)
	}
	pkt = append(pkt, 0x00)
	tail := make([]byte, 4)
	binary.BigEndian.PutUint16(tail[0:2], 1) // QTYPE=A
	binary.BigEndian.PutUint16(tail[2:4], 1) // QCLASS=IN
	return append(pkt, tail...), nil
}

func skipDNSName(msg []byte, offset int) (int, error) {
	for offset < len(msg) {
		b := msg[offset]
		if b == 0 {
			return offset + 1, nil
		}
		if b&0xc0 == 0xc0 {
			if offset+2 > len(msg) {
				return 0, fmt.Errorf("truncated pointer")
			}
			return offset + 2, nil
		}
		step := int(b) + 1
		if offset+step > len(msg) {
			return 0, fmt.Errorf("truncated label")
		}
		offset += step
	}
	return 0, fmt.Errorf("unterminated name")
}

func parseDNSARecordAnswers(msg []byte) ([]net.IPAddr, error) {
	if len(msg) < 12 {
		return nil, fmt.Errorf("short dns response")
	}
	if msg[3]&0x0f != 0 {
		return nil, fmt.Errorf("dns rcode %d", msg[3]&0x0f)
	}
	qdCount := int(binary.BigEndian.Uint16(msg[4:6]))
	anCount := int(binary.BigEndian.Uint16(msg[6:8]))
	offset := 12
	for i := 0; i < qdCount; i++ {
		next, err := skipDNSName(msg, offset)
		if err != nil || next+4 > len(msg) {
			return nil, fmt.Errorf("malformed question")
		}
		offset = next + 4
	}

	var out []net.IPAddr
	for i := 0; i < anCount; i++ {
		next, err := skipDNSName(msg, offset)
		if err != nil || next+10 > len(msg) {
			break
		}
		rrType := binary.BigEndian.Uint16(msg[next : next+2])
		rdLen := int(binary.BigEndian.Uint16(msg[next+8 : next+10]))
		rdataStart := next + 10
		if rdataStart+rdLen > len(msg) {
			break
		}
		if rrType == 1 && rdLen == 4 {
			ip := net.IPv4(msg[rdataStart], msg[rdataStart+1], msg[rdataStart+2], msg[rdataStart+3])
			out = append(out, net.IPAddr{IP: ip})
		}
		offset = rdataStart + rdLen
	}
	return out, nil
}

// startInProcessCleanDNSProxy 启动绑定在 127.0.0.1 随机端口的进程内防 Fake-IP DNS 代理（供 net.DefaultResolver / tsnet STUN 共用）
func startInProcessCleanDNSProxy() (string, error) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	go func() {
		buf := make([]byte, 512)
		for {
			n, clientAddr, readErr := pc.ReadFrom(buf)
			if readErr != nil {
				return
			}
			if n < 12 {
				continue
			}
			pkt := make([]byte, n)
			copy(pkt, buf[:n])
			go handleCleanDNSProxyPacket(pc, clientAddr, pkt)
		}
	}()
	return pc.LocalAddr().String(), nil
}

func handleCleanDNSProxyPacket(pc net.PacketConn, clientAddr net.Addr, pkt []byte) {
	qEnd, host, qtype, err := extractQuestionFromPacket(pkt)
	if err != nil {
		return
	}

	// 非 A 记录直接返回 NODATA，避免双栈等待超时
	if qtype != 1 {
		resp := make([]byte, qEnd)
		copy(resp, pkt[:qEnd])
		resp[2] = 0x81
		resp[3] = 0x80
		binary.BigEndian.PutUint16(resp[6:8], 0)
		_, _ = pc.WriteTo(resp, clientAddr)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	ips, resolveErr := Default.ResolveIPAddr(ctx, host)
	cancel()

	resp := make([]byte, qEnd)
	copy(resp, pkt[:qEnd])
	resp[2] = 0x81
	if resolveErr != nil || len(ips) == 0 {
		resp[3] = 0x83 // NXDOMAIN
		_, _ = pc.WriteTo(resp, clientAddr)
		return
	}

	resp[3] = 0x80
	var count uint16
	for _, item := range ips {
		ip4 := item.IP.To4()
		if ip4 == nil {
			continue
		}
		ans := []byte{
			0xc0, 0x0c,
			0x00, 0x01,
			0x00, 0x01,
			0x00, 0x00, 0x00, 0x1e,
			0x00, 0x04,
			ip4[0], ip4[1], ip4[2], ip4[3],
		}
		resp = append(resp, ans...)
		count++
	}
	binary.BigEndian.PutUint16(resp[6:8], count)
	_, _ = pc.WriteTo(resp, clientAddr)
}

func extractQuestionFromPacket(pkt []byte) (int, string, uint16, error) {
	offset := 12
	var labels []string
	for offset < len(pkt) {
		l := int(pkt[offset])
		if l == 0 {
			offset++
			break
		}
		offset++
		if offset+l > len(pkt) {
			return 0, "", 0, fmt.Errorf("label overflow")
		}
		labels = append(labels, string(pkt[offset:offset+l]))
		offset += l
	}
	if offset+4 > len(pkt) {
		return 0, "", 0, fmt.Errorf("question tail overflow")
	}
	qtype := binary.BigEndian.Uint16(pkt[offset : offset+2])
	return offset + 4, strings.Join(labels, "."), qtype, nil
}

// DialContext 全局安全 TCP 拨号入口（供 gRPC 与 HTTP Transport 共用）
func DialContext(ctx context.Context, addr string) (net.Conn, error) {
	return Default.DialContext(ctx, addr)
}

// EnsureProcessSafeResolverForTarget 对目标控制面及代表性 STUN 域名做一次 Fake-IP 探测；
// 若当前环境存在 Proxifier (127.x) 或 OpenClash (198.18.0.0/15) Fake-IP 劫持，则升级进程级默认解析器为防劫持干净解析器。
func EnsureProcessSafeResolverForTarget(rawTarget string) {
	probeHosts := []string{}
	if h := ExtractHost(rawTarget); h != "" && net.ParseIP(h) == nil {
		probeHosts = append(probeHosts, h)
	}
	probeHosts = append(probeHosts, "stun.qq.com", "stun.miwifi.com")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	hijacked := false
	for _, h := range probeHosts {
		osIPs, err := rawOSResolver.LookupIPAddr(ctx, h)
		if err == nil && IsSyntheticLoopbackFakeIP(h, osIPs) {
			hijacked = true
			_, _ = Default.ResolveIPAddr(ctx, h)
			break
		}
	}
	if !hijacked {
		return
	}

	processResolverOnce.Do(func() {
		proxyAddr, err := startInProcessCleanDNSProxy()
		if err != nil {
			log.Printf("[NetUtil] 启动进程内防 Fake-IP DNS 代理失败: %v", err)
			return
		}
		localCleanDNSAddr = proxyAddr
		*net.DefaultResolver = net.Resolver{
			PreferGo: true,
			Dial: func(dialCtx context.Context, _, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: 2 * time.Second}
				return d.DialContext(dialCtx, "udp", localCleanDNSAddr)
			},
		}
		if tr, ok := http.DefaultTransport.(*http.Transport); ok {
			tr.DialContext = func(dialCtx context.Context, _, addr string) (net.Conn, error) {
				return Default.DialContext(dialCtx, addr)
			}
		}
		log.Printf("[NetUtil] 已启用进程级防 Fake-IP 干净解析器（本地入口: %s, 验证上游: %s），tsnet STUN 与 HTTP 同步免疫", localCleanDNSAddr, Default.VerifiedDNSServer())
	})
}
