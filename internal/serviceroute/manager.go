package serviceroute

import (
	"errors"
	"fmt"
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
	localPort             int
	targetPort            int
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
			resource.LocalError = ""
			desired[resource.Domain] = resource
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for domain, current := range m.routes {
		if _, exists := desired[domain]; !exists {
			m.proxy.StopSVCProxy(current.vip, current.localPort)
			m.allocator.Release(domain)
			delete(m.routes, domain)
		}
	}
	var syncErrors []error
	for domain, resource := range desired {
		localPort := int(resource.LocalPort)
		if localPort == 0 {
			localPort = int(resource.Port)
		}
		current, exists := m.routes[domain]
		if exists && current.tenantID == resource.TenantID && current.resourceID == resource.ResourceID && current.targetRevision == resource.TargetRevision &&
			current.authorizationRevision == resource.AuthorizationRevision && current.sessionID == resource.SessionID &&
			current.sourceID == resource.SourceID && current.targetRevisionID == resource.TargetRevisionID &&
			current.agentIP == resource.AgentIP && current.svcProxyPort == resource.SVCProxyPort &&
			current.namespace == resource.Namespace && current.serviceUID == resource.ServiceUID &&
			current.serviceName == resource.ServiceName && current.portName == resource.PortName &&
			current.protocol == resource.Protocol && current.localPort == localPort && current.targetPort == int(resource.Port) {
			continue
		}
		vip, err := m.allocator.Allocate(domain)
		if err != nil {
			if exists {
				m.proxy.StopSVCProxy(current.vip, current.localPort)
				m.allocator.Release(domain)
				delete(m.routes, domain)
			}
			resource.LocalError = fmt.Sprintf("本地访问地址分配失败，请重试：%v", err)
			syncErrors = append(syncErrors, fmt.Errorf("%s: %w", domain, err))
			continue
		}
		if exists {
			m.proxy.StopSVCProxy(current.vip, current.localPort)
			delete(m.routes, domain)
		}
		target := proxy.SVCTarget{
			Domain: domain, VIP: vip, Port: localPort, AgentIP: resource.AgentIP, GRPCPort: int(resource.SVCProxyPort),
			Namespace: resource.Namespace, ServiceName: resource.ServiceName, TargetPort: int(resource.Port),
			SessionID: resource.SessionID, ResourceID: resource.ResourceID, SourceID: resource.SourceID,
			TargetRevisionID: resource.TargetRevisionID, ServiceUID: resource.ServiceUID,
			PortName: resource.PortName, Protocol: resource.Protocol, AuthorizationRevision: resource.AuthorizationRevision,
		}
		if err := m.proxy.StartSVCProxy(target); err != nil {
			m.allocator.Release(domain)
			resource.LocalError = fmt.Sprintf("本地端口 %d 无法监听，请为当前设备更换本地端口：%v", localPort, err)
			syncErrors = append(syncErrors, fmt.Errorf("%s:%d: %w", domain, localPort, err))
			continue
		}
		m.routes[domain] = route{
			tenantID: resource.TenantID, resourceID: resource.ResourceID, targetRevision: resource.TargetRevision,
			authorizationRevision: resource.AuthorizationRevision, sessionID: resource.SessionID,
			sourceID: resource.SourceID, targetRevisionID: resource.TargetRevisionID,
			agentIP: resource.AgentIP, svcProxyPort: resource.SVCProxyPort,
			namespace: resource.Namespace, serviceUID: resource.ServiceUID, serviceName: resource.ServiceName,
			portName: resource.PortName, protocol: resource.Protocol, vip: vip, localPort: localPort, targetPort: int(resource.Port),
		}
	}
	return errors.Join(syncErrors...)
}

func validResource(resource *client.ResourceInfo) bool {
	return resource != nil && resource.Type == "container_service" && resource.TenantID != "" && resource.ResourceID != "" && resource.Domain != "" &&
		resource.AgentIP != "" && resource.SVCProxyPort > 0 && resource.Namespace != "" && resource.ServiceUID != "" &&
		resource.ServiceName != "" && resource.Port > 0 && resource.Port <= 65535 && resource.LocalPort >= 0 && resource.LocalPort <= 65535 && resource.Protocol == "TCP" &&
		resource.SessionID != "" && resource.SourceID != "" && resource.TargetRevisionID != "" &&
		resource.TargetRevision > 0 && resource.AuthorizationRevision > 0
}
