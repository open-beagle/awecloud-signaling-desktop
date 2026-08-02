package serviceroute

import (
	"sync"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
	"github.com/open-beagle/awecloud-signaling-desktop/internal/proxy"
)

type Allocator interface {
	Allocate(string) (string, error)
	Release(string)
}

type ProxyManager interface {
	StartSVCProxy(proxy.SVCTarget) error
	StopSVCProxy(string, int)
}

type route struct {
	tenantID              string
	resourceID            string
	targetRevision        int64
	authorizationRevision int64
	sessionID             string
	sourceID              string
	targetRevisionID      string
	agentIP               string
	svcProxyPort          uint32
	namespace             string
	serviceUID            string
	serviceName           string
	portName              string
	protocol              string
	vip                   string
	port                  int
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
		if validResource(resource) {
			desired[resource.Domain] = resource
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for domain, current := range m.routes {
		if _, exists := desired[domain]; !exists {
			m.proxy.StopSVCProxy(current.vip, current.port)
			m.allocator.Release(domain)
			delete(m.routes, domain)
		}
	}
	for domain, resource := range desired {
		current, exists := m.routes[domain]
		if exists && current.tenantID == resource.TenantID && current.resourceID == resource.ResourceID && current.targetRevision == resource.TargetRevision &&
			current.authorizationRevision == resource.AuthorizationRevision && current.sessionID == resource.SessionID &&
			current.sourceID == resource.SourceID && current.targetRevisionID == resource.TargetRevisionID &&
			current.agentIP == resource.AgentIP && current.svcProxyPort == resource.SVCProxyPort &&
			current.namespace == resource.Namespace && current.serviceUID == resource.ServiceUID &&
			current.serviceName == resource.ServiceName && current.portName == resource.PortName &&
			current.protocol == resource.Protocol && current.port == int(resource.Port) {
			continue
		}
		vip, err := m.allocator.Allocate(domain)
		if err != nil {
			return err
		}
		if exists {
			m.proxy.StopSVCProxy(current.vip, current.port)
		}
		target := proxy.SVCTarget{
			Domain: domain, VIP: vip, Port: int(resource.Port), AgentIP: resource.AgentIP, GRPCPort: int(resource.SVCProxyPort),
			Namespace: resource.Namespace, ServiceName: resource.ServiceName, TargetPort: int(resource.Port),
			SessionID: resource.SessionID, ResourceID: resource.ResourceID, SourceID: resource.SourceID,
			TargetRevisionID: resource.TargetRevisionID, ServiceUID: resource.ServiceUID,
			PortName: resource.PortName, Protocol: resource.Protocol, AuthorizationRevision: resource.AuthorizationRevision,
		}
		if err := m.proxy.StartSVCProxy(target); err != nil {
			return err
		}
		m.routes[domain] = route{
			tenantID: resource.TenantID, resourceID: resource.ResourceID, targetRevision: resource.TargetRevision,
			authorizationRevision: resource.AuthorizationRevision, sessionID: resource.SessionID,
			sourceID: resource.SourceID, targetRevisionID: resource.TargetRevisionID,
			agentIP: resource.AgentIP, svcProxyPort: resource.SVCProxyPort,
			namespace: resource.Namespace, serviceUID: resource.ServiceUID, serviceName: resource.ServiceName,
			portName: resource.PortName, protocol: resource.Protocol, vip: vip, port: int(resource.Port),
		}
	}
	return nil
}

func validResource(resource *client.ResourceInfo) bool {
	return resource != nil && resource.Type == "container_service" && resource.TenantID != "" && resource.ResourceID != "" && resource.Domain != "" &&
		resource.AgentIP != "" && resource.SVCProxyPort > 0 && resource.Namespace != "" && resource.ServiceUID != "" &&
		resource.ServiceName != "" && resource.Port > 0 && resource.Port <= 65535 && resource.Protocol == "TCP" &&
		resource.SessionID != "" && resource.SourceID != "" && resource.TargetRevisionID != "" &&
		resource.TargetRevision > 0 && resource.AuthorizationRevision > 0
}
