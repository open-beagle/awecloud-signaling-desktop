package netutil

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeServerEndpoint(t *testing.T) {
	hostPort, useTLS := NormalizeServerEndpoint("https://signal.example.com/")
	require.Equal(t, "signal.example.com:443", hostPort)
	require.True(t, useTLS)

	hostPort, useTLS = NormalizeServerEndpoint("https://signal.example.com:8443")
	require.Equal(t, "signal.example.com:8443", hostPort)
	require.True(t, useTLS)

	hostPort, useTLS = NormalizeServerEndpoint("http://signal.example.com")
	require.Equal(t, "signal.example.com:80", hostPort)
	require.False(t, useTLS)

	hostPort, useTLS = NormalizeServerEndpoint("localhost:8080")
	require.Equal(t, "localhost:8080", hostPort)
	require.False(t, useTLS)
}

func TestIsSyntheticLoopbackFakeIP(t *testing.T) {
	fake := []net.IPAddr{{IP: net.ParseIP("127.107.0.92")}}
	loopback1 := []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}
	lan := []net.IPAddr{{IP: net.ParseIP("192.168.1.20")}}
	tunFake := []net.IPAddr{{IP: net.ParseIP("198.18.0.15")}}

	require.True(t, IsSyntheticLoopbackFakeIP("signal.example.com", fake))
	require.False(t, IsSyntheticLoopbackFakeIP("signal.example.com", loopback1))
	require.False(t, IsSyntheticLoopbackFakeIP("signal.example.com", lan))
	require.True(t, IsSyntheticLoopbackFakeIP("signal.example.com", tunFake))
	require.False(t, IsSyntheticLoopbackFakeIP("localhost", fake))
	require.False(t, IsSyntheticLoopbackFakeIP("home-pc.rdp.beagle", []net.IPAddr{{IP: net.ParseIP("127.1.0.2")}}))
	require.False(t, IsSyntheticLoopbackFakeIP("127.107.0.92", fake))

	t.Cleanup(func() { SetActiveDomainSuffix(DefaultDomainSuffix) })
	require.Equal(t, ".acme", DeriveBrandDomainSuffix("https://signal.acme.io"))
	require.Equal(t, ".acme", DeriveBrandDomainSuffix("https://signal.acme.com.cn:8443"))
	require.Equal(t, ".beagle", DeriveBrandDomainSuffix("http://127.0.0.1:8080"))
	require.Equal(t, ".beagle", DeriveBrandDomainSuffix("localhost:8080"))
	SetActiveDomainSuffix(".acme")
	require.False(t, IsSyntheticLoopbackFakeIP("home.alice.acme", []net.IPAddr{{IP: net.ParseIP("127.1.0.5")}}))
}

func TestResolverBypassesSyntheticLoopbackViaUDP(t *testing.T) {
	dnsAddr := startMockARecordDNSServer(t, net.IPv4(203, 0, 113, 25))

	r := &DirectResolver{
		osLookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("127.107.0.92")}}, nil
		},
		adapterDNS: func() []string {
			return []string{dnsAddr}
		},
		fallbackDNS: nil,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ips, err := r.ResolveIPAddr(ctx, "signal.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, ips)
	require.Equal(t, "203.0.113.25", ips[0].IP.String())
	require.Equal(t, dnsAddr, r.VerifiedDNSServer())

	// OpenClash 198.18.0.0/15 Fake-IP 场景：路由器 DNS 同样返回 198.18.0.15 时应自动跳过路由器并从干净公网 DNS 解析真实 IP
	poisonedGatewayDNS := startMockARecordDNSServer(t, net.IPv4(198, 18, 0, 15))
	rOpenClash := &DirectResolver{
		osLookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("198.18.0.15")}}, nil
		},
		adapterDNS: func() []string {
			return []string{poisonedGatewayDNS}
		},
		fallbackDNS: []string{dnsAddr},
	}
	ipsClash, err := rOpenClash.ResolveIPAddr(ctx, "headscale.example.com")
	require.NoError(t, err)
	require.NotEmpty(t, ipsClash)
	require.Equal(t, "203.0.113.25", ipsClash[0].IP.String())
}

func TestDialContextConnectsRealTargetWhenOSReturnsSyntheticLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	accepted := make(chan struct{}, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
			accepted <- struct{}{}
		}
	}()

	_, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)

	// Mock UDP DNS returns 127.0.0.1 (real listener IP in test), while OS lookup returns poisoned 127.107.0.92.
	dnsAddr := startMockARecordDNSServer(t, net.IPv4(127, 0, 0, 1))
	r := &DirectResolver{
		osLookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("127.107.0.92")}}, nil
		},
		adapterDNS: func() []string {
			return []string{dnsAddr}
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := r.DialContext(ctx, net.JoinHostPort("signal.example.com", port))
	require.NoError(t, err)
	_ = conn.Close()

	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("expected TCP listener to receive connection")
	}
}

func startMockARecordDNSServer(t *testing.T, answerIP net.IP) string {
	t.Helper()
	ip4 := answerIP.To4()
	require.NotNil(t, ip4)

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = pc.Close() })

	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if n < 12 {
				continue
			}
			// Find end of QNAME + QTYPE(2) + QCLASS(2)
			idx := 12
			for idx < n && buf[idx] != 0 {
				idx += int(buf[idx]) + 1
			}
			if idx+5 > n {
				continue
			}
			qEnd := idx + 5
			qType := binary.BigEndian.Uint16(buf[idx+1 : idx+3])

			resp := make([]byte, 0, qEnd+16)
			resp = append(resp, buf[:qEnd]...)
			resp[2] = 0x81
			resp[3] = 0x80
			if qType == 1 { // A record
				binary.BigEndian.PutUint16(resp[6:8], 1)
				// Pointer to QNAME (0xc00c), TYPE A (1), CLASS IN (1), TTL 60, RDLENGTH 4, IPv4
				ans := []byte{
					0xc0, 0x0c,
					0x00, 0x01,
					0x00, 0x01,
					0x00, 0x00, 0x00, 0x3c,
					0x00, 0x04,
					ip4[0], ip4[1], ip4[2], ip4[3],
				}
				resp = append(resp, ans...)
			} else {
				// NODATA for AAAA so LookupIPAddr completes cleanly without error
				binary.BigEndian.PutUint16(resp[6:8], 0)
			}
			_, _ = pc.WriteTo(resp, addr)
		}
	}()

	return pc.LocalAddr().String()
}
