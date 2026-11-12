// Package dns 提供本地 DNS 服务器，拦截 .beagle 域名解析
// 监听 127.0.0.1:15353，将 .beagle 域名解析为 VIP 地址（127.1.x.x）
// 非 .beagle 域名转发到上游 DNS
package dns

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
)

// ResolveFunc 域名解析回调函数
// 输入域名（不含末尾点），返回 VIP 地址和是否成功
type ResolveFunc func(domain string) (vip string, ok bool)

// Server 本地 DNS 服务器
type Server struct {
	listenAddr  string
	boundIP     string
	boundPort   int
	conn        *net.UDPConn
	resolve     ResolveFunc
	upstreamDNS string // 上游 DNS 地址（用于转发非受管内网域名）

	suffixMu     sync.RWMutex
	domainSuffix string

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewServer 创建 DNS 服务器
func NewServer(listenAddr string, resolve ResolveFunc) *Server {
	return &Server{
		listenAddr:   listenAddr,
		resolve:      resolve,
		upstreamDNS:  "223.5.5.5:53",
		domainSuffix: ".beagle",
		stopCh:       make(chan struct{}),
	}
}

// SetDomainSuffix 动态设置当前受管内网域名根后缀（同时始终兼容 .beagle）
func (s *Server) SetDomainSuffix(suffix string) {
	trimmed := strings.ToLower(strings.TrimSpace(suffix))
	trimmed = strings.TrimSuffix(trimmed, ".")
	if trimmed != "" && !strings.HasPrefix(trimmed, ".") {
		trimmed = "." + trimmed
	}
	if trimmed == "" || trimmed == "." {
		trimmed = ".beagle"
	}
	s.suffixMu.Lock()
	s.domainSuffix = trimmed
	s.suffixMu.Unlock()
}

// DomainSuffix 返回当前主受管域名根后缀
func (s *Server) DomainSuffix() string {
	s.suffixMu.RLock()
	defer s.suffixMu.RUnlock()
	if s.domainSuffix == "" {
		return ".beagle"
	}
	return s.domainSuffix
}

func (s *Server) matchesManagedSuffix(domain string) bool {
	lower := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if strings.HasSuffix(lower, ".beagle") {
		return true
	}
	s.suffixMu.RLock()
	suffix := s.domainSuffix
	s.suffixMu.RUnlock()
	return suffix != "" && strings.HasSuffix(lower, suffix)
}

// ListenAddr 返回实际绑定生效的 UDP 监听地址
func (s *Server) ListenAddr() string {
	if s.boundIP != "" && s.boundPort > 0 {
		return fmt.Sprintf("%s:%d", s.boundIP, s.boundPort)
	}
	return s.listenAddr
}

// ListenIP 返回实际绑定的本地回环 IP
func (s *Server) ListenIP() string {
	if s.boundIP != "" {
		return s.boundIP
	}
	host, _, err := net.SplitHostPort(s.listenAddr)
	if err == nil && host != "" {
		return host
	}
	return "127.0.0.2"
}

// ListenPort 返回实际绑定的 UDP 端口
func (s *Server) ListenPort() int {
	if s.boundPort > 0 {
		return s.boundPort
	}
	return RecommendedPort()
}

// Start 启动 DNS 服务器（若 127.0.0.2:53 被同机其他实例占用，自动顺延尝试 127.0.0.3..127.0.0.9:53）
func (s *Server) Start() error {
	candidates := []string{s.listenAddr}
	if host, port, err := net.SplitHostPort(s.listenAddr); err == nil && host == "127.0.0.2" && port == "53" {
		for lastOctet := 3; lastOctet <= 9; lastOctet++ {
			candidates = append(candidates, fmt.Sprintf("127.0.0.%d:53", lastOctet))
		}
	}

	var conn *net.UDPConn
	var lastErr error
	for _, candidate := range candidates {
		addr, err := net.ResolveUDPAddr("udp", candidate)
		if err != nil {
			lastErr = fmt.Errorf("解析监听地址失败: %w", err)
			continue
		}
		c, err := net.ListenUDP("udp", addr)
		if err != nil {
			lastErr = err
			continue
		}
		conn = c
		s.listenAddr = candidate
		if udpAddr, ok := c.LocalAddr().(*net.UDPAddr); ok && udpAddr != nil {
			s.boundIP = udpAddr.IP.String()
			s.boundPort = udpAddr.Port
		}
		break
	}
	if conn == nil {
		return fmt.Errorf("监听 DNS 端口失败: %w", lastErr)
	}

	s.conn = conn
	log.Printf("[DNS] 本地 DNS 服务器已启动: %s", s.ListenAddr())

	s.wg.Add(1)
	go s.serve()

	return nil
}

// Stop 停止 DNS 服务器
func (s *Server) Stop() {
	close(s.stopCh)
	if s.conn != nil {
		s.conn.Close()
	}
	s.wg.Wait()
	log.Printf("[DNS] 本地 DNS 服务器已停止")
}

// serve 处理 DNS 请求
func (s *Server) serve() {
	defer s.wg.Done()

	buf := make([]byte, 512)
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		n, remoteAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
				log.Printf("[DNS] 读取请求失败: %v", err)
				continue
			}
		}

		// 异步处理请求
		packet := make([]byte, n)
		copy(packet, buf[:n])
		go s.handleQuery(packet, remoteAddr)
	}
}

// handleQuery 处理单个 DNS 查询
func (s *Server) handleQuery(packet []byte, remoteAddr *net.UDPAddr) {
	// 解析查询域名
	domain, qtype, err := parseDNSQuestion(packet)
	if err != nil {
		log.Printf("[DNS] 解析查询失败: %v", err)
		return
	}

	// 去掉末尾的点
	domain = strings.TrimSuffix(domain, ".")

	// 检查是否是受管零信任内网域名（含动态品牌后缀与 .beagle）
	if s.matchesManagedSuffix(domain) && qtype == 1 { // A 记录
		vip, ok := s.resolve(domain)
		if ok {
			resp := buildDNSResponse(packet, vip)
			s.conn.WriteToUDP(resp, remoteAddr)
			log.Printf("[DNS] 解析: %s → %s", domain, vip)
			return
		}
		// 域名未注册，返回 NXDOMAIN
		resp := buildDNSNXDomain(packet)
		s.conn.WriteToUDP(resp, remoteAddr)
		log.Printf("[DNS] 域名未注册: %s", domain)
		return
	}

	// 非 .beagle 域名，转发到上游 DNS
	resp, err := s.forwardToUpstream(packet)
	if err != nil {
		log.Printf("[DNS] 转发到上游失败: %v", err)
		resp = buildDNSServerFailure(packet)
	}
	s.conn.WriteToUDP(resp, remoteAddr)
}

// forwardToUpstream 转发查询到上游 DNS
func (s *Server) forwardToUpstream(packet []byte) ([]byte, error) {
	upstreamAddr, err := net.ResolveUDPAddr("udp", s.upstreamDNS)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", nil, upstreamAddr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err := conn.Write(packet); err != nil {
		return nil, err
	}

	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}

	return buf[:n], nil
}

// parseDNSQuestion 从 DNS 报文中解析查询域名和类型
func parseDNSQuestion(packet []byte) (string, uint16, error) {
	if len(packet) < 12 {
		return "", 0, fmt.Errorf("报文太短")
	}

	// 跳过 DNS 头部（12 字节）
	offset := 12
	var parts []string

	for offset < len(packet) {
		length := int(packet[offset])
		if length == 0 {
			offset++
			break
		}
		offset++
		if offset+length > len(packet) {
			return "", 0, fmt.Errorf("域名标签越界")
		}
		parts = append(parts, string(packet[offset:offset+length]))
		offset += length
	}

	if offset+4 > len(packet) {
		return "", 0, fmt.Errorf("查询类型越界")
	}

	qtype := binary.BigEndian.Uint16(packet[offset : offset+2])
	domain := strings.Join(parts, ".")

	return domain, qtype, nil
}

// buildDNSResponse 构建 DNS A 记录响应
func buildDNSResponse(query []byte, ip string) []byte {
	parsedIP := net.ParseIP(ip).To4()
	if parsedIP == nil {
		return buildDNSServerFailure(query)
	}

	// 复制查询报文作为响应基础
	resp := make([]byte, len(query))
	copy(resp, query)

	// 设置响应标志
	resp[2] = 0x81 // QR=1, Opcode=0, AA=1
	resp[3] = 0x80 // RA=1, RCODE=0 (No Error)

	// 设置 Answer Count = 1
	resp[6] = 0x00
	resp[7] = 0x01

	// 追加 Answer 段
	// Name: 指针指向 Question 中的域名（offset 12）
	answer := []byte{
		0xc0, 0x0c, // 名称指针
		0x00, 0x01, // Type: A
		0x00, 0x01, // Class: IN
		0x00, 0x00, 0x00, 0x3c, // TTL: 60 秒
		0x00, 0x04, // RDLENGTH: 4
	}
	answer = append(answer, parsedIP...)

	return append(resp, answer...)
}

// buildDNSNXDomain 构建 NXDOMAIN 响应
func buildDNSNXDomain(query []byte) []byte {
	resp := make([]byte, len(query))
	copy(resp, query)
	resp[2] = 0x81 // QR=1, AA=1
	resp[3] = 0x83 // RA=1, RCODE=3 (NXDOMAIN)
	return resp
}

// buildDNSServerFailure 构建 Server Failure 响应
func buildDNSServerFailure(query []byte) []byte {
	resp := make([]byte, len(query))
	copy(resp, query)
	resp[2] = 0x81 // QR=1, AA=1
	resp[3] = 0x82 // RA=1, RCODE=2 (Server Failure)
	return resp
}
