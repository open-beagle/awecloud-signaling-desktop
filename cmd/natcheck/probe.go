package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/netutil"
)

const (
	stunBindingRequest  = 0x0001
	stunBindingResponse = 0x0101
	stunMagicCookie     = 0x2112A442
	stunAttrMappedAddr  = 0x0001
	stunAttrXorMapped   = 0x0020
	stunAttrSoftware    = 0x8022
	stunAttrFingerprint = 0x8028
	stunFingerprintXOR  = 0x5354554e
)

// FirewallStatus 描述本机防火墙与网络配置文件对测试版 Desktop UDP 入站的放行状态
type FirewallStatus struct {
	OS                    string   `json:"os"`
	IsElevated            bool     `json:"is_elevated"`
	ActiveProfiles        []string `json:"active_profiles"`
	TargetBinaryExists    bool     `json:"target_binary_exists"`
	HasExplicitBlockUDP   bool     `json:"has_explicit_block_udp"`
	HasAllowUDPForProfile bool     `json:"has_allow_udp_for_profile"`
	AllowRuleProfiles     []string `json:"allow_rule_profiles"`
	Verdict               string   `json:"verdict"` // allow / blocked / missing_rule_public / unknown
	Summary               string   `json:"summary"`
}

// STUNObservation 单次 STUN 探测脱敏观测结果（严禁保存真实公网 IP 或私有域名）
type STUNObservation struct {
	ServerLabel string `json:"server_label"`
	Reachable   bool   `json:"reachable"`
	RTTMs       int64  `json:"rtt_ms"`
	IPScope     string `json:"ip_scope"`     // public-v4 / cgnat-v4 / private-v4 / global-v6 / unknown
	IPBucket    string `json:"ip_bucket"`    // 同一次运行内的匿名桶编号，如 ip#1
	MappedPort  int    `json:"mapped_port"`  // 路由器映射后的外部 UDP 端口
	ErrorReason string `json:"error_reason"` // 超时或错误原因（脱敏）
}

// NATReport 完整 UDP 打洞与 NAT 可行性脱敏诊断报告
type NATReport struct {
	GeneratedAtUTC      string            `json:"generated_at_utc"`
	LocalUDPPort        int               `json:"local_udp_port"`
	ControlSTUNTested   bool              `json:"control_stun_tested"`
	ControlSTUNReachable bool             `json:"control_stun_reachable"`
	Observations        []STUNObservation `json:"observations"`
	RespondedCount      int               `json:"responded_count"`
	NATMappingType      string            `json:"nat_mapping_type"` // cone / symmetric_port / multi_egress_ip / insufficient_stun
	PortSpread          int               `json:"port_spread"`
	PreservesSourcePort bool              `json:"preserves_source_port"`
	HasGlobalIPv6       bool              `json:"has_global_ipv6"`
	IPv6STUNReachable   bool              `json:"ipv6_stun_reachable"`
	UPnPIGDAvailable    bool              `json:"upnp_igd_available"`
	Firewall            FirewallStatus    `json:"firewall"`
	SupportsUDPPunching string            `json:"supports_udp_punching"` // yes / conditional / unlikely
	PrimaryBottlenecks  []string          `json:"primary_bottlenecks"`
	Recommendations     []string          `json:"recommendations"`
}

type stunTarget struct {
	Label    string
	HostPort string
	IsCustom bool
}

var defaultPublicSTUNTargets = []stunTarget{
	{Label: "public-stun-1", HostPort: "stun.miwifi.com:3478"},
	{Label: "public-stun-2", HostPort: "stun.qq.com:3478"},
	{Label: "public-stun-3", HostPort: "stun.cloudflare.com:3478"},
	{Label: "public-stun-4", HostPort: "stun.syncthing.net:3478"},
}

// BuildTailscaleSTUNRequest 构造带 SOFTWARE 与 RFC 5389 FINGERPRINT 的标准 STUN Binding Request
// Headscale 内置 Tailscale STUN 服务端强校验 FINGERPRINT 属性，普通裸 20 字节包会被静默丢弃。
func BuildTailscaleSTUNRequest(txID [12]byte) []byte {
	sw := []byte("tailnode")
	padLen := (4 - (len(sw) % 4)) % 4
	swAttrLen := 4 + len(sw) + padLen
	fpAttrLen := 8 // header(4) + crc32(4)
	msgLenWithFP := swAttrLen + fpAttrLen

	pkt := make([]byte, 20+swAttrLen, 20+msgLenWithFP)
	binary.BigEndian.PutUint16(pkt[0:2], stunBindingRequest)
	binary.BigEndian.PutUint16(pkt[2:4], uint16(msgLenWithFP))
	binary.BigEndian.PutUint32(pkt[4:8], stunMagicCookie)
	copy(pkt[8:20], txID[:])

	binary.BigEndian.PutUint16(pkt[20:22], stunAttrSoftware)
	binary.BigEndian.PutUint16(pkt[22:24], uint16(len(sw)))
	copy(pkt[24:24+len(sw)], sw)

	crc := crc32.ChecksumIEEE(pkt) ^ stunFingerprintXOR
	fp := make([]byte, 8)
	binary.BigEndian.PutUint16(fp[0:2], stunAttrFingerprint)
	binary.BigEndian.PutUint16(fp[2:4], 4)
	binary.BigEndian.PutUint32(fp[4:8], crc)
	return append(pkt, fp...)
}

// ParseSTUNBindingResponse 解析 STUN Binding Response 中的外部映射 IP 与端口
func ParseSTUNBindingResponse(pkt []byte, expectedTxID [12]byte) (net.IP, int, error) {
	if len(pkt) < 20 {
		return nil, 0, fmt.Errorf("short stun packet (%d bytes)", len(pkt))
	}
	msgType := binary.BigEndian.Uint16(pkt[0:2])
	msgLen := int(binary.BigEndian.Uint16(pkt[2:4]))
	cookie := binary.BigEndian.Uint32(pkt[4:8])
	if msgType != stunBindingResponse {
		return nil, 0, fmt.Errorf("unexpected stun type 0x%04x", msgType)
	}
	if cookie != stunMagicCookie {
		return nil, 0, fmt.Errorf("invalid stun magic cookie")
	}
	var gotTx [12]byte
	copy(gotTx[:], pkt[8:20])
	if gotTx != expectedTxID {
		return nil, 0, fmt.Errorf("stun transaction id mismatch")
	}
	end := 20 + msgLen
	if end > len(pkt) {
		end = len(pkt)
	}

	var fallbackIP net.IP
	var fallbackPort int

	offset := 20
	for offset+4 <= end {
		attrType := binary.BigEndian.Uint16(pkt[offset : offset+2])
		attrLen := int(binary.BigEndian.Uint16(pkt[offset+2 : offset+4]))
		valStart := offset + 4
		valEnd := valStart + attrLen
		if valEnd > end {
			break
		}
		val := pkt[valStart:valEnd]

		switch attrType {
		case stunAttrXorMapped:
			ip, port, err := decodeXorMappedAddress(val, expectedTxID)
			if err == nil {
				return ip, port, nil
			}
		case stunAttrMappedAddr:
			ip, port, err := decodePlainMappedAddress(val)
			if err == nil && fallbackIP == nil {
				fallbackIP = ip
				fallbackPort = port
			}
		}

		padded := (attrLen + 3) & ^3
		offset = valStart + padded
	}

	if fallbackIP != nil {
		return fallbackIP, fallbackPort, nil
	}
	return nil, 0, fmt.Errorf("no mapped address attribute in stun response")
}

func decodeXorMappedAddress(val []byte, txID [12]byte) (net.IP, int, error) {
	if len(val) < 8 {
		return nil, 0, fmt.Errorf("short xor-mapped-address")
	}
	family := val[1]
	port := int(binary.BigEndian.Uint16(val[2:4]) ^ uint16(stunMagicCookie>>16))
	switch family {
	case 0x01: // IPv4
		var cookieBytes [4]byte
		binary.BigEndian.PutUint32(cookieBytes[:], stunMagicCookie)
		ip := net.IPv4(
			val[4]^cookieBytes[0],
			val[5]^cookieBytes[1],
			val[6]^cookieBytes[2],
			val[7]^cookieBytes[3],
		)
		return ip, port, nil
	case 0x02: // IPv6
		if len(val) < 20 {
			return nil, 0, fmt.Errorf("short ipv6 xor-mapped-address")
		}
		var mask [16]byte
		binary.BigEndian.PutUint32(mask[0:4], stunMagicCookie)
		copy(mask[4:16], txID[:])
		ip := make(net.IP, 16)
		for i := 0; i < 16; i++ {
			ip[i] = val[4+i] ^ mask[i]
		}
		return ip, port, nil
	default:
		return nil, 0, fmt.Errorf("unknown address family 0x%02x", family)
	}
}

func decodePlainMappedAddress(val []byte) (net.IP, int, error) {
	if len(val) < 8 {
		return nil, 0, fmt.Errorf("short mapped-address")
	}
	family := val[1]
	port := int(binary.BigEndian.Uint16(val[2:4]))
	switch family {
	case 0x01:
		return net.IPv4(val[4], val[5], val[6], val[7]), port, nil
	case 0x02:
		if len(val) < 20 {
			return nil, 0, fmt.Errorf("short ipv6 mapped-address")
		}
		ip := make(net.IP, 16)
		copy(ip, val[4:20])
		return ip, port, nil
	default:
		return nil, 0, fmt.Errorf("unknown family 0x%02x", family)
	}
}

// ClassifyIPScope 仅输出地址所属标准网络类型，绝不输出任何真实 IP 字节
func ClassifyIPScope(ip net.IP) string {
	if ip == nil {
		return "unknown"
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4.IsLoopback() {
			return "loopback-v4"
		}
		if ip4.IsPrivate() {
			return "private-v4"
		}
		// RFC 6598 CGNAT 100.64.0.0/10
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return "cgnat-v4"
		}
		return "public-v4"
	}
	if ip.IsLoopback() {
		return "loopback-v6"
	}
	if ip.IsGlobalUnicast() && !ip.IsPrivate() {
		return "global-v6"
	}
	return "local-v6"
}

type observedMapping struct {
	ServerLabel string
	IPKey       string
	IPScope     string
	Port        int
	RTTMs       int64
}

// AnalyzeNATMappings 根据同一本地 UDP 套接字对多台独立 STUN 服务器的外部映射结果判定 NAT 类型
// 若同时包含局域网/回环发夹 NAT（private-v4 / loopback-v4）与外部公网/CGNAT 观测结果，
// 仅对外部 WAN 映射（public-v4 / cgnat-v4）计算 NAT 类型，避免内网网关回环干扰 Cone NAT 判定。
func AnalyzeNATMappings(localPort int, mappings []observedMapping) (natType string, portSpread int, preservesPort bool) {
	wanMappings := make([]observedMapping, 0, len(mappings))
	for _, m := range mappings {
		if m.IPScope == "public-v4" || m.IPScope == "cgnat-v4" {
			wanMappings = append(wanMappings, m)
		}
	}
	effective := mappings
	if len(wanMappings) >= 2 || (len(wanMappings) == 1 && len(mappings) > 1) {
		effective = wanMappings
	}

	if len(effective) < 2 {
		if len(effective) == 1 {
			preservesPort = localPort > 0 && effective[0].Port == localPort
		}
		return "insufficient_stun", 0, preservesPort
	}

	ipSet := make(map[string]struct{})
	ports := make([]int, 0, len(effective))
	allPreserve := true
	for _, m := range effective {
		ipSet[m.IPKey] = struct{}{}
		ports = append(ports, m.Port)
		if localPort <= 0 || m.Port != localPort {
			allPreserve = false
		}
	}
	sort.Ints(ports)
	portSpread = ports[len(ports)-1] - ports[0]

	if len(ipSet) > 1 {
		return "multi_egress_ip", portSpread, false
	}
	if portSpread == 0 {
		return "cone", 0, allPreserve
	}
	return "symmetric_port", portSpread, false
}

// SynthesizeVerdict 结合 NAT 映射特征、Windows 防火墙规则、IPv6 与 UPnP 给出综合打洞结论与建议
func SynthesizeVerdict(r *NATReport) {
	r.PrimaryBottlenecks = nil
	r.Recommendations = nil

	if r.ControlSTUNTested && !r.ControlSTUNReachable {
		r.PrimaryBottlenecks = append(r.PrimaryBottlenecks, "本机到控制面 STUN (UDP 3479) 探测超时，Headscale 无法发现本机实时外部 UDP 端点")
		r.Recommendations = append(r.Recommendations, "检查本机或上级路由器是否拦截出站 UDP 3479")
	}

	switch r.Firewall.Verdict {
	case "blocked":
		r.PrimaryBottlenecks = append(r.PrimaryBottlenecks, "Windows Defender 防火墙存在针对测试版 awecloud-signaling-desktop.exe 的入站 UDP 阻止（Block）规则")
		r.Recommendations = append(r.Recommendations, "在管理员 PowerShell 运行 `go run ./cmd/natcheck --fix-firewall` 移除阻止规则并放行测试版客户端 UDP 入站")
	case "missing_rule_public":
		r.PrimaryBottlenecks = append(r.PrimaryBottlenecks, "当前网卡处于公用网络（Public），但测试版 awecloud-signaling-desktop.exe 尚未注册 Windows 防火墙 UDP 入站放行规则，对端打洞回包会被系统静默丢弃")
		r.Recommendations = append(r.Recommendations, "在管理员 PowerShell 运行 `go run ./cmd/natcheck --fix-firewall`（或执行计划 U2 命令）放行 UDP 入站后重启测试版客户端")
	}

	switch r.NATMappingType {
	case "cone":
		if r.Firewall.Verdict == "blocked" || r.Firewall.Verdict == "missing_rule_public" {
			r.SupportsUDPPunching = "conditional"
			r.Recommendations = append(r.Recommendations, "路由器 NAT 类型为 Cone NAT（外部端口恒定，天然支持 WireGuard UDP 打洞）；放行 Windows 防火墙 UDP 入站后即可建立 P2P 直连")
		} else {
			r.SupportsUDPPunching = "yes"
			r.Recommendations = append(r.Recommendations, "本机路由器为 Cone NAT 且防火墙已放行 UDP 入站，支持常规 WireGuard P2P UDP 打洞直连")
		}
	case "symmetric_port":
		r.PrimaryBottlenecks = append(r.PrimaryBottlenecks, fmt.Sprintf("上级宽带/光猫为对称型 NAT（Symmetric NAT，同一本地端口向不同服务器映射的外部端口差值为 %d）", r.PortSpread))
		if r.HasGlobalIPv6 && r.IPv6STUNReachable {
			r.SupportsUDPPunching = "conditional"
			r.Recommendations = append(r.Recommendations, "IPv4 处于对称型 NAT，但本机具备可用公网 IPv6（UDP6 可达）：当主控端同样处于 IPv6 网络时可直接通过 IPv6 建立低延迟 P2P 直连")
		} else if r.UPnPIGDAvailable {
			r.SupportsUDPPunching = "conditional"
			r.Recommendations = append(r.Recommendations, "IPv4 处于对称型 NAT，但内网网关开启了 UPnP IGD 端口映射；对端若为 Cone NAT（如已实测的出差端）仍可通过 UPnP / 单边 Cone 辅助尝试打洞")
		} else {
			r.SupportsUDPPunching = "conditional"
			r.Recommendations = append(r.Recommendations, "IPv4 为对称型 NAT 且无公网 IPv6：若对端为 Cone NAT，放行本机 Windows 防火墙 UDP 入站后可尝试单边 Cone 穿透；若光猫拨号建议改为路由桥接或开启路由器 IPv6 / UPnP，或在境内增设低延迟 DERP 节点")
		}
	case "multi_egress_ip":
		r.PrimaryBottlenecks = append(r.PrimaryBottlenecks, "运营商出口存在多公网 IP 轮询（CGNAT 多出口负载均衡），常规 IPv4 UDP 打洞极易失配")
		if r.HasGlobalIPv6 && r.IPv6STUNReachable {
			r.SupportsUDPPunching = "conditional"
			r.Recommendations = append(r.Recommendations, "建议优先使用本机已具备的公网 IPv6 直连，或在家庭路由器开启桥接/固定出口")
		} else {
			r.SupportsUDPPunching = "unlikely"
			r.Recommendations = append(r.Recommendations, "建议在家庭光猫/路由器开启 IPv6，或在境内就近部署自建 DERP 中继以降低中转延迟")
		}
	default:
		if r.HasGlobalIPv6 && r.IPv6STUNReachable {
			r.SupportsUDPPunching = "conditional"
			r.Recommendations = append(r.Recommendations, "IPv4 STUN 样本不足（部分 UDP 3478 被拦截），但 IPv6 UDP 可达")
		} else {
			r.SupportsUDPPunching = "unlikely"
			r.PrimaryBottlenecks = append(r.PrimaryBottlenecks, "可用 STUN 响应不足 2 个，疑似当前网络拦截出站/入站 UDP 流量")
		}
	}
}

func discoverSignalingServerHost(explicitURL string) string {
	if strings.TrimSpace(explicitURL) != "" {
		return netutil.ExtractHost(explicitURL)
	}
	if envVal := strings.TrimSpace(os.Getenv("SIGNALING_ADDRESS")); envVal != "" {
		return netutil.ExtractHost(envVal)
	}
	candidates := []string{
		".env",
		filepath.Join("desktop", ".env"),
		filepath.Join("..", "desktop", ".env"),
		filepath.Join("..", ".env"),
	}
	var fallbackHost string
	for _, p := range candidates {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key := strings.TrimSpace(k)
			val := strings.Trim(strings.TrimSpace(v), `"'`)
			switch key {
			case "SIGNALING_ADDRESS":
				if h := netutil.ExtractHost(val); h != "" {
					_ = f.Close()
					return h
				}
			case "STAGING_HEADSCALE_URL", "STAGING_SIGNAL_URL":
				if fallbackHost == "" {
					fallbackHost = netutil.ExtractHost(val)
				}
			}
		}
		_ = f.Close()
	}
	return fallbackHost
}

func resolveUDP4Addr(ctx context.Context, hostPort string) (*net.UDPAddr, error) {
	host, portStr, err := net.SplitHostPort(hostPort)
	if err != nil {
		return nil, err
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil || port <= 0 {
		return nil, fmt.Errorf("invalid port")
	}
	ips, err := netutil.Default.ResolveIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, item := range ips {
		if ip4 := item.IP.To4(); ip4 != nil {
			return &net.UDPAddr{IP: ip4, Port: port}, nil
		}
	}
	return nil, fmt.Errorf("no ipv4 address resolved")
}

func runSingleSocketSTUNProbes(ctx context.Context, controlHost string) (int, []STUNObservation, []observedMapping, bool, bool) {
	targets := make([]stunTarget, 0, len(defaultPublicSTUNTargets)+1)
	controlTested := false
	if controlHost != "" {
		controlTested = true
		targets = append(targets, stunTarget{
			Label:    "control-plane-stun:3479",
			HostPort: net.JoinHostPort(controlHost, "3479"),
			IsCustom: true,
		})
	}
	targets = append(targets, defaultPublicSTUNTargets...)

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return 0, nil, nil, controlTested, false
	}
	defer conn.Close()

	localPort := 0
	if ua, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		localPort = ua.Port
	}

	ipBuckets := make(map[string]string)
	nextBucket := 1
	bucketFor := func(ip net.IP) string {
		key := ip.String()
		if b, ok := ipBuckets[key]; ok {
			return b
		}
		b := fmt.Sprintf("ip#%d", nextBucket)
		nextBucket++
		ipBuckets[key] = b
		return b
	}

	observations := make([]STUNObservation, 0, len(targets))
	mappings := make([]observedMapping, 0, len(targets))
	controlOK := false

	for _, t := range targets {
		obs := STUNObservation{ServerLabel: t.Label}
		rCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
		udpAddr, rErr := resolveUDP4Addr(rCtx, t.HostPort)
		cancel()
		if rErr != nil {
			obs.ErrorReason = "dns_resolve_failed"
			observations = append(observations, obs)
			continue
		}

		var txID [12]byte
		_, _ = rand.Read(txID[:])
		req := BuildTailscaleSTUNRequest(txID)

		start := time.Now()
		_ = conn.SetReadDeadline(time.Now().Add(1800 * time.Millisecond))
		if _, wErr := conn.WriteToUDP(req, udpAddr); wErr != nil {
			obs.ErrorReason = "udp_send_failed"
			observations = append(observations, obs)
			continue
		}

		var mappedIP net.IP
		var mappedPort int
		var rtt time.Duration
		buf := make([]byte, 1024)
		for {
			n, _, readErr := conn.ReadFromUDP(buf)
			if readErr != nil {
				obs.ErrorReason = "timeout"
				break
			}
			ip, p, pErr := ParseSTUNBindingResponse(buf[:n], txID)
			if pErr == nil {
				mappedIP = ip
				mappedPort = p
				rtt = time.Since(start)
				break
			}
		}

		if mappedIP != nil {
			obs.Reachable = true
			obs.RTTMs = rtt.Milliseconds()
			obs.IPScope = ClassifyIPScope(mappedIP)
			obs.IPBucket = bucketFor(mappedIP)
			obs.MappedPort = mappedPort
			if t.IsCustom {
				controlOK = true
			}
			mappings = append(mappings, observedMapping{
				ServerLabel: t.Label,
				IPKey:       obs.IPBucket,
				IPScope:     obs.IPScope,
				Port:        mappedPort,
				RTTMs:       obs.RTTMs,
			})
		}
		observations = append(observations, obs)
	}

	return localPort, observations, mappings, controlTested, controlOK
}

func probeGlobalIPv6(ctx context.Context) (hasGlobalV6 bool, v6STUNOK bool) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false, false
	}
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok || ipNet.IP == nil || ipNet.IP.To4() != nil {
			continue
		}
		if ipNet.IP.IsGlobalUnicast() && !ipNet.IP.IsPrivate() {
			hasGlobalV6 = true
			break
		}
	}
	if !hasGlobalV6 {
		return false, false
	}

	for _, hostPort := range []string{"stun.cloudflare.com:3478", "stun.qq.com:3478"} {
		host, _, _ := net.SplitHostPort(hostPort)
		rCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		ips, err := netutil.Default.ResolveIPAddr(rCtx, host)
		cancel()
		if err != nil {
			continue
		}
		var targetIP net.IP
		for _, item := range ips {
			if item.IP.To4() == nil && item.IP.IsGlobalUnicast() {
				targetIP = item.IP
				break
			}
		}
		if targetIP == nil {
			continue
		}
		conn, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6zero, Port: 0})
		if err != nil {
			return hasGlobalV6, false
		}
		var txID [12]byte
		_, _ = rand.Read(txID[:])
		req := BuildTailscaleSTUNRequest(txID)
		_ = conn.SetDeadline(time.Now().Add(1500 * time.Millisecond))
		_, wErr := conn.WriteToUDP(req, &net.UDPAddr{IP: targetIP, Port: 3478})
		if wErr == nil {
			buf := make([]byte, 1024)
			if n, _, rErr := conn.ReadFromUDP(buf); rErr == nil {
				if _, _, pErr := ParseSTUNBindingResponse(buf[:n], txID); pErr == nil {
					_ = conn.Close()
					return true, true
				}
			}
		}
		_ = conn.Close()
	}
	return hasGlobalV6, false
}

func probeUPnPIGD() bool {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return false
	}
	defer conn.Close()

	ssdpAddr := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}
	searchTargets := []string{
		"urn:schemas-upnp-org:service:WANIPConnection:1",
		"urn:schemas-upnp-org:service:WANPPPConnection:1",
		"urn:schemas-upnp-org:device:InternetGatewayDevice:1",
	}
	for _, st := range searchTargets {
		payload := fmt.Sprintf("M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: %s\r\n\r\n", st)
		_, _ = conn.WriteToUDP([]byte(payload), ssdpAddr)
	}

	_ = conn.SetReadDeadline(time.Now().Add(1200 * time.Millisecond))
	buf := make([]byte, 2048)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			return false
		}
		resp := strings.ToLower(string(buf[:n]))
		if strings.Contains(resp, "wanipconnection") ||
			strings.Contains(resp, "wanpppconnection") ||
			strings.Contains(resp, "internetgatewaydevice") {
			return true
		}
	}
}

// RenderMarkdownReport 输出 100% 脱敏的 Markdown 诊断摘要（最多两列表格，符合仓库排版规范）
func RenderMarkdownReport(r NATReport) string {
	var b strings.Builder
	b.WriteString("### 远端 Windows UDP 打洞与 NAT 可行性诊断报告（脱敏）\n\n")
	b.WriteString("| 诊断维度 | 检测结果 |\n")
	b.WriteString("| :--- | :--- |\n")
	b.WriteString(fmt.Sprintf("| 诊断时间（UTC） | `%s` |\n", r.GeneratedAtUTC))
	b.WriteString(fmt.Sprintf("| 综合 UDP 打洞可行性 | **`%s`** |\n", r.SupportsUDPPunching))
	b.WriteString(fmt.Sprintf("| IPv4 NAT 映射类型 | `%s`（外部端口跨度 `%d`，保源端口 `%v`） |\n", r.NATMappingType, r.PortSpread, r.PreservesSourcePort))
	if r.ControlSTUNTested {
		b.WriteString(fmt.Sprintf("| 控制面 STUN (`3479/UDP`) | 可达 `%v` |\n", r.ControlSTUNReachable))
	}
	b.WriteString(fmt.Sprintf("| STUN 成功响应数 | `%d / %d`（本地源端口 `%d`） |\n", r.RespondedCount, len(r.Observations), r.LocalUDPPort))
	b.WriteString(fmt.Sprintf("| 公网 IPv6 直连能力 | 全局单播 `%v` · UDP6 STUN 可达 `%v` |\n", r.HasGlobalIPv6, r.IPv6STUNReachable))
	b.WriteString(fmt.Sprintf("| 路由器 UPnP IGD 映射 | `%v` |\n", r.UPnPIGDAvailable))
	b.WriteString(fmt.Sprintf("| Windows 防火墙 UDP 入站 | `%s`（活动网络 `%s`） |\n\n", r.Firewall.Verdict, strings.Join(r.Firewall.ActiveProfiles, ",")))

	b.WriteString("#### STUN 多点同一套接字观测明细（匿名脱敏）\n\n")
	for _, obs := range r.Observations {
		if obs.Reachable {
			b.WriteString(fmt.Sprintf("- `%s`：可达，RTT `%d ms`，地址类型 `%s`（`%s`），外部映射端口 `%d`\n",
				obs.ServerLabel, obs.RTTMs, obs.IPScope, obs.IPBucket, obs.MappedPort))
		} else {
			b.WriteString(fmt.Sprintf("- `%s`：不可达（`%s`）\n", obs.ServerLabel, obs.ErrorReason))
		}
	}

	if len(r.PrimaryBottlenecks) > 0 {
		b.WriteString("\n#### 识别到的打洞阻碍项\n\n")
		for _, item := range r.PrimaryBottlenecks {
			b.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}

	if len(r.Recommendations) > 0 {
		b.WriteString("\n#### 处置建议\n\n")
		for _, item := range r.Recommendations {
			b.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}
	return b.String()
}
