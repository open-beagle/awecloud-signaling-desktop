package dns

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func buildTestDNSQuery(domain string, qtype uint16) []byte {
	pkt := make([]byte, 12)
	binary.BigEndian.PutUint16(pkt[0:2], 0x4242)
	binary.BigEndian.PutUint16(pkt[2:4], 0x0100) // RD=1
	binary.BigEndian.PutUint16(pkt[4:6], 1)      // QDCOUNT=1

	for _, label := range splitLabels(domain) {
		pkt = append(pkt, byte(len(label)))
		pkt = append(pkt, []byte(label)...)
	}
	pkt = append(pkt, 0x00)

	tail := make([]byte, 4)
	binary.BigEndian.PutUint16(tail[0:2], qtype)
	binary.BigEndian.PutUint16(tail[2:4], 1) // IN
	return append(pkt, tail...)
}

func splitLabels(domain string) []string {
	var out []string
	cur := ""
	for i := 0; i < len(domain); i++ {
		if domain[i] == '.' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(domain[i])
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func TestServerManagedSuffixHandlesAAndNoDataWithoutUpstreamLeak(t *testing.T) {
	// 创建一个故意不响应的黑洞 UDP 端口模拟上游阻塞，确保受管后缀的 AAAA 查询绝不泄露或卡住上游
	blackholeConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	defer blackholeConn.Close()

	srv := NewServer("127.0.0.1:0", func(domain string) (string, bool) {
		if domain == "home.alice.acme" {
			return "127.1.0.9", true
		}
		return "", false
	})
	srv.upstreamDNS = blackholeConn.LocalAddr().String()
	srv.SetDomainSuffix(".acme")
	require.NoError(t, srv.Start())
	defer srv.Stop()

	serverAddr, err := net.ResolveUDPAddr("udp", srv.ListenAddr())
	require.NoError(t, err)
	clientConn, err := net.DialUDP("udp", nil, serverAddr)
	require.NoError(t, err)
	defer clientConn.Close()

	// 1. A 记录命中 -> RCODE=0, ANCOUNT=1, IP=127.1.0.9
	require.NoError(t, clientConn.SetDeadline(time.Now().Add(300*time.Millisecond)))
	_, err = clientConn.Write(buildTestDNSQuery("home.alice.acme", 1))
	require.NoError(t, err)
	buf := make([]byte, 512)
	n, err := clientConn.Read(buf)
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, 16)
	require.Equal(t, byte(0), buf[3]&0x0f)
	require.Equal(t, uint16(1), binary.BigEndian.Uint16(buf[6:8]))
	require.Equal(t, []byte{127, 1, 0, 9}, buf[n-4:n])

	// 2. AAAA 记录 (qtype=28) 针对受管后缀 -> 必须在极短时间内直接返回 NODATA (RCODE=0, ANCOUNT=0)，绝不转发黑洞上游
	start := time.Now()
	require.NoError(t, clientConn.SetDeadline(time.Now().Add(200*time.Millisecond)))
	_, err = clientConn.Write(buildTestDNSQuery("home.alice.acme", 28))
	require.NoError(t, err)
	n, err = clientConn.Read(buf)
	require.NoError(t, err, "受管后缀的 AAAA 查询不应转发至上游导致超时")
	require.Less(t, time.Since(start), 150*time.Millisecond)
	require.Equal(t, byte(0), buf[3]&0x0f, "NODATA 响应 RCODE 应为 0")
	require.Equal(t, uint16(0), binary.BigEndian.Uint16(buf[6:8]), "NODATA 响应 ANCOUNT 应为 0")

	// 3. 未注册受管域名 A 记录 -> NXDOMAIN (RCODE=3)
	require.NoError(t, clientConn.SetDeadline(time.Now().Add(300*time.Millisecond)))
	_, err = clientConn.Write(buildTestDNSQuery("unknown.alice.acme", 1))
	require.NoError(t, err)
	n, err = clientConn.Read(buf)
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, 12)
	require.Equal(t, byte(3), buf[3]&0x0f)

	// 4. 携带 EDNS0 OPT 附加记录 (ARCOUNT=1) 的查询 -> 响应必须剥离 OPT 并保持严格 Question -> Answer 顺序 (ARCOUNT=0)
	baseQuery := buildTestDNSQuery("home.alice.acme", 1)
	ednsQuery := make([]byte, len(baseQuery))
	copy(ednsQuery, baseQuery)
	binary.BigEndian.PutUint16(ednsQuery[10:12], 1) // ARCOUNT=1
	// 标准 11 字节 EDNS0 OPT 伪记录 (Name=root 0x00, Type=41 0x0029, UDP size=4096 0x1000)
	ednsOPT := []byte{0x00, 0x00, 0x29, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	ednsQuery = append(ednsQuery, ednsOPT...)

	require.NoError(t, clientConn.SetDeadline(time.Now().Add(300*time.Millisecond)))
	_, err = clientConn.Write(ednsQuery)
	require.NoError(t, err)
	n, err = clientConn.Read(buf)
	require.NoError(t, err)
	// 期望长度严格等于 baseQuery (Header+Question) + 16 字节 A Answer，不含 11 字节残留 OPT
	require.Equal(t, len(baseQuery)+16, n)
	require.Equal(t, uint16(1), binary.BigEndian.Uint16(buf[4:6]), "QDCOUNT=1")
	require.Equal(t, uint16(1), binary.BigEndian.Uint16(buf[6:8]), "ANCOUNT=1")
	require.Equal(t, uint16(0), binary.BigEndian.Uint16(buf[8:10]), "NSCOUNT=0")
	require.Equal(t, uint16(0), binary.BigEndian.Uint16(buf[10:12]), "ARCOUNT=0")
	require.Equal(t, []byte{0xc0, 0x0c}, buf[len(baseQuery):len(baseQuery)+2], "Answer 紧接 Question 段之后")
	require.Equal(t, []byte{127, 1, 0, 9}, buf[n-4:n])

	// 5. 内置金丝雀探针域名 _dns-check.acme -> 无需业务回调即可直接返回 127.0.0.1
	require.NoError(t, clientConn.SetDeadline(time.Now().Add(300*time.Millisecond)))
	_, err = clientConn.Write(buildTestDNSQuery(CanaryDomainForSuffix(".acme"), 1))
	require.NoError(t, err)
	n, err = clientConn.Read(buf)
	require.NoError(t, err)
	require.Equal(t, byte(0), buf[3]&0x0f)
	require.Equal(t, uint16(1), binary.BigEndian.Uint16(buf[6:8]))
	require.Equal(t, []byte{127, 0, 0, 1}, buf[n-4:n])
}
