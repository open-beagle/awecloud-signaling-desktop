//go:build !windows

package netutil

func systemAdapterDNSServers() []string {
	return nil
}
