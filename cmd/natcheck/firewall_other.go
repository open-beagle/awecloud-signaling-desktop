//go:build !windows

package main

import "runtime"

func inspectFirewallStatus() FirewallStatus {
	return FirewallStatus{
		OS:      runtime.GOOS,
		Verdict: "allow",
		Summary: "非 Windows 平台无需 Windows Defender 应用程序入站规则检查",
	}
}

func applyStagingUDPFirewallFix() error {
	return nil
}
