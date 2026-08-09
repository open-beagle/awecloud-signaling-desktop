//go:build s6real

package app

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/open-beagle/awecloud-signaling-desktop/internal/client"
)

func TestS6TenantSwitchReal(t *testing.T) {
	server := mustEnv(t, "S6_SERVER")
	desktopID, err := strconv.ParseUint(mustEnv(t, "S6_DESKTOP_ID"), 10, 64)
	if err != nil {
		t.Fatalf("invalid S6_DESKTOP_ID: %v", err)
	}
	secret := mustEnv(t, "S6_DESKTOP_SECRET")
	tenantA := mustEnv(t, "S6_TENANT_A")
	tenantB := mustEnv(t, "S6_TENANT_B")

	desktopClient := client.NewDesktopClient(server)
	if err := desktopClient.Start(); err != nil {
		t.Fatalf("start Desktop client: %v", err)
	}
	auth, err := desktopClient.Authenticate(desktopID, secret)
	if err != nil {
		desktopClient.Stop()
		t.Fatalf("authenticate Desktop: %v", err)
	}
	app := &App{desktopClient: desktopClient, authResult: auth}
	t.Cleanup(app.teardownLocalSession)
	if err := app.initializeTailscale(); err != nil {
		t.Fatalf("initialize tunnel: %v", err)
	}
	time.Sleep(3 * time.Second)

	resourcesA := switchAndValidate(t, app, tenantA, "tenant-a")
	oldProxy := app.proxyManager
	oldSVCProxy := app.svcProxyMgr
	oldDomains := resourceDomains(resourcesA)

	resourcesB := switchAndValidate(t, app, tenantB, "tenant-b")
	assertDeparted(t, app, oldProxy.Count(), oldSVCProxy.Count(), oldDomains)
	oldProxy = app.proxyManager
	oldSVCProxy = app.svcProxyMgr
	oldDomains = resourceDomains(resourcesB)

	_ = switchAndValidate(t, app, tenantA, "tenant-a")
	assertDeparted(t, app, oldProxy.Count(), oldSVCProxy.Count(), oldDomains)
	fmt.Println("PASS real_tenant_switch=A->B->A dns=nrpt vip=recreated listeners=stopped openSSH=exit37 services=2")
}

func switchAndValidate(t *testing.T, app *App, tenantID, markerPrefix string) []*client.ResourceInfo {
	t.Helper()
	resources, err := app.SwitchResourceTenant(tenantID)
	if err != nil {
		t.Fatalf("switch Tenant %s: %v", tenantID, err)
	}
	var sshResource *client.ResourceInfo
	services := make([]*client.ResourceInfo, 0, 2)
	for _, resource := range resources {
		if resource == nil || resource.TenantID != tenantID {
			t.Fatalf("cross-Tenant resource in %s snapshot", tenantID)
		}
		switch resource.Type {
		case "container_ssh":
			sshResource = resource
		case "container_service":
			services = append(services, resource)
		}
	}
	if sshResource == nil || len(services) != 2 || len(resources) != 3 {
		t.Fatalf("unexpected %s resource shape: total=%d ssh=%v services=%d", tenantID, len(resources), sshResource != nil, len(services))
	}
	if app.proxyManager == nil || app.proxyManager.Count() != 1 {
		t.Fatalf("ContainerSSH listener count is not 1")
	}
	if app.svcProxyMgr == nil || app.svcProxyMgr.Count() != 2 {
		t.Fatalf("ContainerService listener count is not 2")
	}
	// ResourceSession snapshots reach the Agent on its short authorization
	// refresh loop; wait one full window before exercising the data plane.
	time.Sleep(4 * time.Second)
	assertNRPT(t)
	assertDNS(t, sshResource.Domain, true)
	assertOpenSSHExit37(t, sshResource)
	sort.Slice(services, func(i, j int) bool { return services[i].Port < services[j].Port })
	for _, resource := range services {
		assertService(t, resource, fmt.Sprintf("%s-port-%d", markerPrefix, resource.Port))
	}
	fmt.Printf("PASS tenant=%s resources=3 ssh_exit=37 service_ports=2 nrpt=active\n", tenantID)
	return resources
}

func assertDeparted(t *testing.T, app *App, oldProxyCount, oldSVCProxyCount int, oldDomains []string) {
	t.Helper()
	if oldProxyCount != 0 || oldSVCProxyCount != 0 {
		t.Fatalf("departed Tenant listeners remain: ssh=%d service=%d", oldProxyCount, oldSVCProxyCount)
	}
	clearDNSCache(t)
	for _, domain := range oldDomains {
		if _, ok := app.resolveDomain(domain); ok {
			t.Fatalf("departed Tenant domain remains allowed: %s", domain)
		}
		assertDNS(t, domain, false)
		if _, ok := app.vipAllocator.GetVIP(domain); ok {
			t.Fatalf("departed Tenant VIP mapping remains: %s", domain)
		}
	}
}

func assertNRPT(t *testing.T) {
	t.Helper()
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", `(Get-DnsClientNrptRule | Where-Object { $_.Namespace -eq '.beagle' -and $_.NameServers -contains '127.0.0.2' }).Count`)
	output, err := cmd.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) == "0" {
		t.Fatalf("Windows NRPT rule is not active: %v", err)
	}
}

func assertDNS(t *testing.T, domain string, shouldResolve bool) {
	t.Helper()
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "Resolve-DnsName -DnsOnly -Type A -Name '"+domain+"' -ErrorAction Stop | Select-Object -ExpandProperty IPAddress")
	output, err := cmd.CombinedOutput()
	resolved := err == nil && strings.Contains(strings.TrimSpace(string(output)), "127.")
	if resolved != shouldResolve {
		t.Fatalf("unexpected Windows DNS result for %s: resolved=%v err=%v", domain, resolved, err)
	}
}

func clearDNSCache(t *testing.T) {
	t.Helper()
	if output, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "Clear-DnsClientCache").CombinedOutput(); err != nil {
		t.Fatalf("clear Windows DNS cache: %v (%s)", err, strings.TrimSpace(string(output)))
	}
}

func assertOpenSSHExit37(t *testing.T, resource *client.ResourceInfo) {
	t.Helper()
	marker := "S6_OPENSSH_EXIT37"
	// Redirected Windows stdin has no console dimensions, so OpenSSH emits a
	// 0x0 pty-req that RFC-aware servers correctly reject. -T still requests
	// an interactive shell and lets this automated probe verify its exit status.
	require.NotEmpty(t, resource.SSHUsers)
	cmd := exec.Command("ssh.exe", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=NUL", "-o", "LogLevel=ERROR", "-T", resource.SSHUsers[0]+"@"+resource.Domain)
	cmd.Stdin = strings.NewReader("printf '" + marker + "\\n'; sleep 2; exit 37\n")
	output, err := cmd.CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 37 {
		t.Fatalf("OpenSSH exit status mismatch: err=%v output=%q", err, strings.TrimSpace(string(output)))
	}
}

func assertService(t *testing.T, resource *client.ResourceInfo, marker string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(resource.Domain, strconv.Itoa(int(resource.Port))), 15*time.Second)
	if err != nil {
		t.Fatalf("dial service %s:%d: %v", resource.Domain, resource.Port, err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != marker {
		t.Fatalf("service marker mismatch: got=%q want=%q err=%v", strings.TrimSpace(line), marker, err)
	}
}

func resourceDomains(resources []*client.ResourceInfo) []string {
	domains := make([]string, 0, len(resources))
	for _, resource := range resources {
		if resource != nil && resource.Domain != "" {
			domains = append(domains, resource.Domain)
		}
	}
	return domains
}

func mustEnv(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("missing environment variable %s", name)
	}
	return value
}
