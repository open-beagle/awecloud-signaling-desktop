package diagnostics

import "tailscale.com/envknob"

// EnableDiscoveryLogging must run at process startup, before creating tsnet.
// os.Setenv alone does not update Tailscale's already registered debug knobs.
func EnableDiscoveryLogging() {
	envknob.Setenv("TS_DEBUG_DISCO", "true")
	Record("network", "[DiscoveryDiagnostics] schema=2 disco=true; local bounded logs enabled")
}
