package app

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf16"

	beaglekubeconfig "github.com/open-beagle/awecloud-signaling-desktop/internal/kubeconfig"
)

type KubeconfigTarget struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Available bool   `json:"available"`
	Hint      string `json:"hint"`
	distro    string
}

type KubeconfigInstallRequest struct {
	Domain    string   `json:"domain"`
	TargetIDs []string `json:"target_ids"`
}

type KubeconfigTargetResult struct {
	TargetID   string `json:"target_id"`
	TargetName string `json:"target_name"`
	Path       string `json:"path"`
	Context    string `json:"context"`
	Success    bool   `json:"success"`
	Error      string `json:"error"`
}

type KubeconfigInstallResult struct {
	Context string                    `json:"context"`
	Targets []*KubeconfigTargetResult `json:"targets"`
}

func (a *App) GetKubeconfigTargets() ([]*KubeconfigTarget, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户目录失败: %w", err)
	}
	localName := "本机"
	localKind := "local"
	if runtime.GOOS == "windows" {
		localName = "Windows"
		localKind = "windows"
	}
	targets := []*KubeconfigTarget{{
		ID:        "local",
		Name:      localName,
		Kind:      localKind,
		Path:      filepath.Join(homeDir, ".kube", "config"),
		Available: true,
		Hint:      "当前系统用户",
	}}
	if runtime.GOOS != "windows" {
		return targets, nil
	}

	distros, err := listWSLDistros()
	if err != nil || len(distros) == 0 {
		hint := "未检测到 WSL，请先安装并配置发行版"
		if err == nil {
			hint = "已安装 WSL，但尚无可用发行版"
		}
		return append(targets, &KubeconfigTarget{
			ID: "wsl", Name: "WSL", Kind: "wsl", Path: "~/.kube/config", Hint: hint,
		}), nil
	}
	for _, distro := range distros {
		home, homeErr := wslHome(distro)
		target := &KubeconfigTarget{
			ID:        "wsl:" + distro,
			Name:      "WSL · " + distro,
			Kind:      "wsl",
			Path:      "~/.kube/config",
			Available: homeErr == nil && home != "",
			Hint:      "发行版用户目录",
			distro:    distro,
		}
		if target.Available {
			target.Path = strings.TrimRight(home, "/") + "/.kube/config"
		} else {
			target.Hint = "无法读取发行版用户目录"
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func (a *App) InstallKubeconfig(request *KubeconfigInstallRequest) (*KubeconfigInstallResult, error) {
	if request == nil || strings.TrimSpace(request.Domain) == "" {
		return nil, fmt.Errorf("请选择要安装的 Kubernetes 集群")
	}
	if len(request.TargetIDs) == 0 {
		return nil, fmt.Errorf("请选择安装环境")
	}

	domains, err := a.GetDomainList()
	if err != nil {
		return nil, err
	}
	clusters := make([]beaglekubeconfig.Cluster, 0)
	currentContext := ""
	for _, domain := range domains {
		if domain == nil || domain.Type != "k8sapi" || !strings.EqualFold(strings.TrimSpace(domain.Status), "online") {
			continue
		}
		name := kubeconfigClusterName(domain)
		clusters = append(clusters, beaglekubeconfig.Cluster{Name: name, Server: kubeconfigServer(domain.Domain)})
		if domain.Domain == request.Domain {
			currentContext = name
		}
	}
	if currentContext == "" {
		return nil, fmt.Errorf("所选 Kubernetes 集群不在线或不在当前授权范围内")
	}
	sort.SliceStable(clusters, func(i, j int) bool { return clusters[i].Name < clusters[j].Name })

	targets, err := a.GetKubeconfigTargets()
	if err != nil {
		return nil, err
	}
	targetByID := make(map[string]*KubeconfigTarget, len(targets))
	for _, target := range targets {
		targetByID[target.ID] = target
	}

	result := &KubeconfigInstallResult{Context: currentContext, Targets: make([]*KubeconfigTargetResult, 0, len(request.TargetIDs))}
	seen := make(map[string]struct{}, len(request.TargetIDs))
	for _, targetID := range request.TargetIDs {
		if _, duplicate := seen[targetID]; duplicate {
			continue
		}
		seen[targetID] = struct{}{}
		target := targetByID[targetID]
		item := &KubeconfigTargetResult{TargetID: targetID, Context: currentContext}
		if target == nil || !target.Available {
			item.Error = "安装环境不可用"
			result.Targets = append(result.Targets, item)
			continue
		}
		item.TargetName = target.Name
		item.Path = target.Path
		existing, readErr := readKubeconfigTarget(target)
		if readErr != nil {
			item.Error = readErr.Error()
			result.Targets = append(result.Targets, item)
			continue
		}
		content, mergeErr := beaglekubeconfig.Merge(existing, clusters, currentContext)
		if mergeErr != nil {
			item.Error = mergeErr.Error()
			result.Targets = append(result.Targets, item)
			continue
		}
		if writeErr := writeKubeconfigTarget(target, content); writeErr != nil {
			item.Error = writeErr.Error()
			result.Targets = append(result.Targets, item)
			continue
		}
		item.Success = true
		result.Targets = append(result.Targets, item)
	}
	return result, nil
}

func kubeconfigClusterName(domain *DomainItem) string {
	region := strings.TrimSpace(domain.Region)
	if region != "" {
		if strings.HasSuffix(region, ".beagle") {
			return region
		}
		return region + ".beagle"
	}
	name := strings.TrimPrefix(strings.TrimSpace(domain.Domain), "kubernetes.")
	name = strings.Split(name, ":")[0]
	if name == "" {
		return "kubernetes.beagle"
	}
	return name
}

func kubeconfigServer(domain string) string {
	domain = strings.TrimSpace(domain)
	if strings.HasPrefix(domain, "https://") || strings.HasPrefix(domain, "http://") {
		return domain
	}
	return "https://" + domain
}

func readKubeconfigTarget(target *KubeconfigTarget) ([]byte, error) {
	if target.Kind != "wsl" {
		content, err := os.ReadFile(target.Path)
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("读取 kubeconfig 失败: %w", err)
		}
		return content, nil
	}
	command := exec.Command("wsl.exe", "-d", target.distro, "--", "sh", "-lc", `if [ -f "$HOME/.kube/config" ]; then cat "$HOME/.kube/config"; fi`)
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("读取 WSL kubeconfig 失败: %w", err)
	}
	return output, nil
}

func writeKubeconfigTarget(target *KubeconfigTarget, content []byte) error {
	if target.Kind != "wsl" {
		return writeLocalKubeconfig(target.Path, content)
	}
	command := exec.Command("wsl.exe", "-d", target.distro, "--", "sh", "-lc", `set -eu; umask 077; mkdir -p "$HOME/.kube"; tmp=$(mktemp "$HOME/.kube/.config.XXXXXX"); trap 'rm -f "$tmp"' EXIT; cat > "$tmp"; chmod 600 "$tmp"; mv -f "$tmp" "$HOME/.kube/config"; trap - EXIT`)
	command.Stdin = bytes.NewReader(content)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("写入 WSL kubeconfig 失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func writeLocalKubeconfig(path string, content []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return fmt.Errorf("创建 .kube 目录失败: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".config-*")
	if err != nil {
		return fmt.Errorf("创建临时 kubeconfig 失败: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return fmt.Errorf("设置 kubeconfig 权限失败: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("写入临时 kubeconfig 失败: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("同步 kubeconfig 失败: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭临时 kubeconfig 失败: %w", err)
	}
	if err := replaceKubeconfigFile(temporaryPath, path); err != nil {
		return fmt.Errorf("替换 kubeconfig 失败: %w", err)
	}
	return nil
}

func listWSLDistros() ([]string, error) {
	if err := exec.Command("wsl.exe", "--status").Run(); err != nil {
		return nil, err
	}
	output, err := exec.Command("wsl.exe", "--list", "--quiet").Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(decodeWindowsCommandOutput(output), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name != "" && !isWSLSystemDistro(name) {
			result = append(result, name)
		}
	}
	return result, nil
}

func isWSLSystemDistro(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	return normalized == "docker-desktop" || normalized == "docker-desktop-data"
}

func wslHome(distro string) (string, error) {
	output, err := exec.Command("wsl.exe", "-d", distro, "--", "sh", "-lc", `printf %s "$HOME"`).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(decodeWindowsCommandOutput(output)), nil
}

func decodeWindowsCommandOutput(input []byte) string {
	if len(input) >= 2 && (input[0] == 0xff && input[1] == 0xfe || bytes.IndexByte(input, 0) >= 0) {
		if len(input)%2 != 0 {
			input = input[:len(input)-1]
		}
		words := make([]uint16, 0, len(input)/2)
		for index := 0; index+1 < len(input); index += 2 {
			word := uint16(input[index]) | uint16(input[index+1])<<8
			if word != 0xfeff {
				words = append(words, word)
			}
		}
		return string(utf16.Decode(words))
	}
	return strings.TrimPrefix(string(input), "\ufeff")
}
