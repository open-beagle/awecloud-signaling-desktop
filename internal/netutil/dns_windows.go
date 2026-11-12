//go:build windows

package netutil

import (
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

func systemAdapterDNSServers() []string {
	flags := uint32(windows.GAA_FLAG_SKIP_ANYCAST | windows.GAA_FLAG_SKIP_MULTICAST | windows.GAA_FLAG_SKIP_FRIENDLY_NAME)
	size := uint32(16384)
	var buf []byte
	for range 3 {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(
			windows.AF_INET,
			flags,
			0,
			(*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])),
			&size,
		)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return nil
		}
	}
	if len(buf) == 0 {
		return nil
	}

	var servers []string
	seen := make(map[string]struct{})
	for aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); aa != nil; aa = aa.Next {
		if aa.OperStatus != windows.IfOperStatusUp || aa.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK {
			continue
		}
		for dns := aa.FirstDnsServerAddress; dns != nil; dns = dns.Next {
			if dns.Address.Sockaddr == nil {
				continue
			}
			rawAny := (*windows.RawSockaddrAny)(unsafe.Pointer(dns.Address.Sockaddr))
			sa, err := rawAny.Sockaddr()
			if err != nil {
				continue
			}
			sa4, ok := sa.(*windows.SockaddrInet4)
			if !ok {
				continue
			}
			ip := net.IP(sa4.Addr[:])
			if ip.IsUnspecified() || ip[0] == 127 {
				continue
			}
			ns := net.JoinHostPort(ip.String(), "53")
			if _, exists := seen[ns]; !exists {
				seen[ns] = struct{}{}
				servers = append(servers, ns)
			}
		}
	}
	return servers
}
