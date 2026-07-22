package containerroute

import (
	"fmt"
	"net"
	"sync"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/proxy"
)

type Allocator interface {
	Allocate(string) (string, error)
	Release(string)
}

type ProxyManager interface {
	StartProxy(proxy.Target) error
	StopProxy(string, int)
}

type route struct {
	resourceID string
	revision   int64
	agentIP    string
	listenPort uint32
	vip        string
}

type Manager struct {
	allocator Allocator
	proxy     ProxyManager
	routes    map[string]route
	mu        sync.Mutex
}

func NewManager(allocator Allocator, proxyManager ProxyManager) *Manager {
	return &Manager{allocator: allocator, proxy: proxyManager, routes: make(map[string]route)}
}

func (m *Manager) Sync(resources []*client.ResourceInfo) error {
	if m == nil || m.allocator == nil || m.proxy == nil {
		return nil
	}
	desired := make(map[string]*client.ResourceInfo)
	for _, resource := range resources {
		if resource != nil && resource.Type == "container_ssh" && resource.Domain != "" && resource.AgentIP != "" && resource.ListenPort > 0 && resource.TargetRevision > 0 {
			desired[resource.Domain] = resource
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for domain, current := range m.routes {
		if _, ok := desired[domain]; !ok {
			m.proxy.StopProxy(current.vip, 22)
			m.allocator.Release(domain)
			delete(m.routes, domain)
		}
	}
	for domain, resource := range desired {
		current, exists := m.routes[domain]
		if exists && current.resourceID == resource.ResourceID && current.revision == resource.TargetRevision && current.agentIP == resource.AgentIP && current.listenPort == resource.ListenPort {
			continue
		}
		vipAddr, err := m.allocator.Allocate(domain)
		if err != nil {
			return err
		}
		if exists {
			m.proxy.StopProxy(current.vip, 22)
		}
		remoteAddr := net.JoinHostPort(resource.AgentIP, fmt.Sprintf("%d", resource.ListenPort))
		if err := m.proxy.StartProxy(proxy.Target{Domain: domain, VIP: vipAddr, RemoteAddr: remoteAddr, Port: 22}); err != nil {
			return err
		}
		m.routes[domain] = route{
			resourceID: resource.ResourceID, revision: resource.TargetRevision,
			agentIP: resource.AgentIP, listenPort: resource.ListenPort, vip: vipAddr,
		}
	}
	return nil
}
