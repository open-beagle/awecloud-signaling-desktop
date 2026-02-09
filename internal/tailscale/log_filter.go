package tailscale

import "strings"

// shouldFilterLog 判断是否应该过滤掉该日志
// 过滤掉过于频繁的 debug 日志，只保留重要信息
func shouldFilterLog(msg string) bool {
	// warming-up 是正常启动状态，不是错误，过滤掉
	if strings.Contains(msg, "warming-up") {
		return true
	}

	// health 日志特殊处理
	if strings.Contains(msg, "health(") {
		if strings.Contains(msg, "): ok") {
			return true
		}
		if strings.Contains(msg, "magicsock-receive-func-error") ||
			strings.Contains(msg, "no-derp-home") {
			return true
		}
		if strings.Contains(msg, "error:") {
			return false
		}
	}

	// 重要日志，不过滤
	importantPatterns := []string{
		"error", "Error", "failed", "Failed",
		"已连接", "已断开", "正在连接",
		"active login",
	}
	for _, pattern := range importantPatterns {
		if strings.Contains(msg, pattern) {
			return false
		}
	}

	// 过滤掉的日志模式
	filterPatterns := []string{
		"monitor: got windows change event",
		"monitor: [unexpected]",
		"monitor: old:", "monitor: new:",
		"InterfaceIPs", "HardwareAddr",
		"wg: [v2]",
		"control: [v1]", "control: [v2]", "control: [vJSON]",
		"netcheck: [v1] report:",
		"router: firewall:", "router: monitorDefaultRoutes",
		"dns: Set:", "dns: Resolvercfg:", "dns: OScfg:",
		"[v1] authReconfig", "[v1] linkChange", "[v1] initPeerAPIListener",
		"tsdial: bart table",
		"magicsock: disco:", "magicsock: [v1]", "magicsock: [v2]",
		"LinkChange:",
		"peerapi: serving",
		"logpolicy:",
		"ipnext: active extensions",
		"blockEngineUpdates",
		"cannot fetch existing TKA state",
		"Switching ipn state",
		"control: NetInfo:", "control: RegisterReq:",
		"control: LoginInteractive", "control: doLogin",
		"control: client.Login", "control: control server key",
		"control: Generating",
		"Start:", "Backend:", "StartLoginInteractive",
		"captive portal detection", "DetectCaptivePortal",
		"controltime",
		// tsnet 特有的日志
		"logtail started",
		"flushing log",
		"[v1] using fake",
		"DNS configurator",
		"OS network configurator",
		"certificate signed by unknown authority",
		"authRoutine:", "TryLogin:", "doLogin(",
		"LoginInteractive", "sendStatus:", "backoff:",
		"fetch control key",
		"magicsock: [warning] failed to force-set UDP",
	}
	for _, pattern := range filterPatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}

	return false
}
