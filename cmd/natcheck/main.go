package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

func main() {
	// 静默底层 netutil 标准日志，确保控制台与报告 100% 不输出任何域名或内网 DNS IP
	log.SetOutput(io.Discard)

	serverFlag := flag.String("server", "", "可选：控制面 HTTPS 地址（默认自动从本地未跟踪 desktop/.env 读取 SIGNALING_ADDRESS）")
	fixFirewall := flag.Bool("fix-firewall", false, "在管理员 PowerShell 下自动为测试版 Desktop 放行 UDP 入站规则")
	jsonOut := flag.Bool("json", false, "以脱敏 JSON 格式输出报告")
	writeReport := flag.String("write-report", "", "可选：将脱敏 Markdown 报告写入指定文件路径")
	flag.Parse()

	if *fixFirewall {
		if err := applyStagingUDPFirewallFix(); err != nil {
			fmt.Fprintf(os.Stderr, "[natcheck] 放行防火墙 UDP 入站失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("[natcheck] 已为测试版 awecloud-signaling-desktop.exe 配置入站 UDP 放行规则（SignalDesktop-Staging-UDP-In）。")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	controlHost := discoverSignalingServerHost(*serverFlag)
	localPort, observations, mappings, controlTested, controlOK := runSingleSocketSTUNProbes(ctx, controlHost)
	natType, portSpread, preservesPort := AnalyzeNATMappings(localPort, mappings)
	hasV6, v6STUN := probeGlobalIPv6(ctx)
	upnpOK := probeUPnPIGD()
	fw := inspectFirewallStatus()

	report := NATReport{
		GeneratedAtUTC:       time.Now().UTC().Format(time.RFC3339),
		LocalUDPPort:         localPort,
		ControlSTUNTested:    controlTested,
		ControlSTUNReachable: controlOK,
		Observations:         observations,
		RespondedCount:       len(mappings),
		NATMappingType:       natType,
		PortSpread:           portSpread,
		PreservesSourcePort:  preservesPort,
		HasGlobalIPv6:        hasV6,
		IPv6STUNReachable:    v6STUN,
		UPnPIGDAvailable:     upnpOK,
		Firewall:             fw,
	}
	SynthesizeVerdict(&report)

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
		return
	}

	md := RenderMarkdownReport(report)
	fmt.Print(md)

	if *writeReport != "" {
		if err := os.WriteFile(*writeReport, []byte(md), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "[natcheck] 写入报告文件失败: %v\n", err)
			os.Exit(1)
		}
	}
}
