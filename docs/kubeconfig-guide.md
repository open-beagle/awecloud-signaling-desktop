# Kubeconfig 配置指南

## 问题说明

当前 Desktop 应用在"我的 K8S"页面点击"复制 kubeconfig"时，会为每个集群生成独立的 kubeconfig 文件内容。这种方式存在以下问题：

1. **管理不便**：每个集群都是独立的配置文件，需要手动合并
2. **切换麻烦**：需要通过 `KUBECONFIG` 环境变量或 `--kubeconfig` 参数指定不同文件
3. **容易混淆**：多个文件分散在不同位置，难以统一管理

## 推荐方案：合并到单一 kubeconfig

### 方案 1：自动合并到 ~/.kube/config（推荐）

应用自动将所有集群配置合并到标准的 `~/.kube/config` 文件中，这是 kubectl 默认读取的位置。

#### 实现逻辑

```go
// GenerateKubeconfig 生成并合并 kubeconfig
func (a *App) GenerateKubeconfig() (*KubeconfigResult, error) {
    // 1. 获取所有 k8sapi 类型的资源
    resources := getK8SAPIResources()
    
    // 2. 为每个资源触发 DNS 解析（分配 VIP + 启动代理）
    clusters := []ClusterConfig{}
    for _, resource := range resources {
        vip := resolveDomain(resource.Domain)
        clusters = append(clusters, ClusterConfig{
            Name:   resource.ClusterName,
            Server: fmt.Sprintf("https://%s:6443", vip),
        })
    }
    
    // 3. 读取现有 kubeconfig
    kubeconfigPath := filepath.Join(os.UserHomeDir(), ".kube", "config")
    existingConfig := readKubeconfig(kubeconfigPath)
    
    // 4. 合并配置（使用标记块）
    mergedConfig := mergeKubeconfig(existingConfig, clusters)
    
    // 5. 写回文件
    writeKubeconfig(kubeconfigPath, mergedConfig)
    
    return &KubeconfigResult{
        Path:     kubeconfigPath,
        Clusters: getClusterNames(clusters),
        Count:    len(clusters),
    }
}
```

#### 合并策略：使用标记块

为了不影响用户已有的 kubeconfig 配置，使用标记块来标识 Signaling 管理的集群：

```yaml
# >>> AWECloud Signaling Clusters >>>
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.1.0.1:6443
    insecure-skip-tls-verify: true
  name: beijing
- cluster:
    server: https://127.1.0.2:6443
    insecure-skip-tls-verify: true
  name: shanghai
contexts:
- context:
    cluster: beijing
    user: signaling-user
  name: beijing
- context:
    cluster: shanghai
    user: signaling-user
  name: shanghai
users:
- name: signaling-user
  user: {}
# <<< AWECloud Signaling Clusters <<<

# 用户原有的配置保持不变
clusters:
- cluster:
    server: https://my-cluster.example.com:6443
  name: my-cluster
...
```

#### 优点

- ✅ kubectl 无需额外配置，直接使用
- ✅ 所有集群在一个文件中，方便管理
- ✅ 使用 `kubectl config use-context <cluster-name>` 快速切换
- ✅ 不影响用户已有配置

### 方案 2：生成独立文件 + 环境变量

如果不想修改用户的 `~/.kube/config`，可以生成独立文件并提示用户设置环境变量。

#### 实现方式

```go
// 生成到独立文件
kubeconfigPath := filepath.Join(os.UserHomeDir(), ".kube", "signaling-config")
writeKubeconfig(kubeconfigPath, config)

// 提示用户设置环境变量
message := fmt.Sprintf(`
Kubeconfig 已生成到: %s

使用方式：
1. 临时使用（当前终端）：
   export KUBECONFIG=%s

2. 永久使用（添加到 ~/.bashrc 或 ~/.zshrc）：
   echo 'export KUBECONFIG=%s' >> ~/.bashrc

3. 合并到默认配置：
   KUBECONFIG=~/.kube/config:%s kubectl config view --flatten > ~/.kube/config.new
   mv ~/.kube/config.new ~/.kube/config
`, kubeconfigPath, kubeconfigPath, kubeconfigPath, kubeconfigPath)
```

#### 优点

- ✅ 不修改用户现有配置
- ✅ 可以独立管理 Signaling 的集群
- ❌ 需要用户手动设置环境变量

### 方案 3：多文件 + KUBECONFIG 合并

利用 kubectl 支持多个 kubeconfig 文件的特性。

#### 使用方式

```bash
# 设置多个 kubeconfig 文件（用冒号分隔）
export KUBECONFIG=~/.kube/config:~/.kube/signaling-config

# kubectl 会自动合并所有文件
kubectl config get-contexts
```

#### 优点

- ✅ 完全隔离，互不影响
- ✅ 可以随时启用/禁用 Signaling 集群
- ❌ 需要用户手动设置环境变量

## 最佳实践建议

### 推荐：方案 1（自动合并）

对于大多数用户，推荐使用方案 1，因为：

1. **零配置**：用户点击"生成 kubeconfig"后立即可用
2. **标准化**：符合 kubectl 的使用习惯
3. **安全性**：使用标记块，不会破坏用户原有配置
4. **易维护**：所有集群在一个文件中，便于查看和管理

### 实现细节

#### 1. 标记块管理

```go
const (
    markerBegin = "# >>> AWECloud Signaling Clusters >>>"
    markerEnd   = "# <<< AWECloud Signaling Clusters <<<"
)

// 查找并替换标记块
func replaceMarkedSection(content string, newSection string) string {
    beginIdx := strings.Index(content, markerBegin)
    endIdx := strings.Index(content, markerEnd)
    
    if beginIdx >= 0 && endIdx >= 0 {
        // 替换现有标记块
        endIdx += len(markerEnd)
        return content[:beginIdx] + newSection + content[endIdx:]
    }
    
    // 追加新标记块
    return content + "\n" + newSection + "\n"
}
```

#### 2. 集群命名规范

从域名提取集群名称：

```
kubernetes.beijing.beagle          → beijing
kubernetes.shanghai.beagle         → shanghai
kubernetes.endpoint-1.beijing.beagle → endpoint-1-beijing
```

#### 3. Context 自动切换

生成 kubeconfig 后，自动切换到第一个集群：

```go
// 设置默认 context
if len(clusters) > 0 {
    exec.Command("kubectl", "config", "use-context", clusters[0].Name).Run()
}
```

### 用户体验优化

#### 前端提示

```vue
<template>
  <div class="kubeconfig-result">
    <div class="success-message">
      ✅ Kubeconfig 已生成并合并到 ~/.kube/config
    </div>
    
    <div class="cluster-list">
      <h4>已配置的集群（{{ clusters.length }}）：</h4>
      <ul>
        <li v-for="cluster in clusters" :key="cluster">
          {{ cluster }}
        </li>
      </ul>
    </div>
    
    <div class="usage-tips">
      <h4>使用方式：</h4>
      <code-block>
        # 查看所有集群
        kubectl config get-contexts
        
        # 切换到指定集群
        kubectl config use-context beijing
        
        # 查看当前集群的 Pod
        kubectl get pods -A
      </code-block>
    </div>
  </div>
</template>
```

#### 错误处理

```go
// 备份原文件
if err := backupKubeconfig(kubeconfigPath); err != nil {
    return nil, fmt.Errorf("备份 kubeconfig 失败: %w", err)
}

// 写入失败时恢复
if err := writeKubeconfig(kubeconfigPath, mergedConfig); err != nil {
    restoreKubeconfig(kubeconfigPath)
    return nil, fmt.Errorf("写入 kubeconfig 失败: %w", err)
}
```

## 常见问题

### Q1: 如何删除 Signaling 生成的集群配置？

**方法 1：通过应用删除**
```
在应用中点击"清理 kubeconfig"按钮，自动删除标记块内的配置
```

**方法 2：手动删除**
```bash
# 编辑 kubeconfig
vim ~/.kube/config

# 删除标记块之间的内容
# >>> AWECloud Signaling Clusters >>>
# ... 删除这部分 ...
# <<< AWECloud Signaling Clusters <<<
```

### Q2: 如何查看 Signaling 管理的集群？

```bash
# 查看所有 context
kubectl config get-contexts

# 筛选 Signaling 的集群（名称通常是地域名）
kubectl config get-contexts | grep -E "beijing|shanghai|guangzhou"
```

### Q3: 如何临时禁用 Signaling 的集群？

```bash
# 切换到其他 context
kubectl config use-context my-other-cluster

# 或者临时使用其他 kubeconfig
kubectl --kubeconfig=/path/to/other/config get pods
```

### Q4: 多个 Desktop 设备如何避免冲突？

每个设备生成的 VIP 地址不同，不会冲突。但集群名称相同，后生成的会覆盖先生成的。

**解决方案**：
- 在不同设备上使用不同的集群名称前缀
- 或者只在一个设备上生成 kubeconfig

## 技术实现参考

### 完整的 YAML 合并代码

```go
package kubeconfig

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

type ClusterConfig struct {
    Name   string
    Server string
    VIP    string
}

type KubeconfigManager struct {
    markerBegin string
    markerEnd   string
}

func NewKubeconfigManager() *KubeconfigManager {
    return &KubeconfigManager{
        markerBegin: "# >>> AWECloud Signaling Clusters >>>",
        markerEnd:   "# <<< AWECloud Signaling Clusters <<<",
    }
}

// MergeToDefaultConfig 合并到默认 kubeconfig
func (m *KubeconfigManager) MergeToDefaultConfig(clusters []ClusterConfig) error {
    homeDir, err := os.UserHomeDir()
    if err != nil {
        return err
    }
    
    kubeconfigPath := filepath.Join(homeDir, ".kube", "config")
    
    // 确保目录存在
    if err := os.MkdirAll(filepath.Dir(kubeconfigPath), 0700); err != nil {
        return err
    }
    
    // 读取现有配置
    existingContent := ""
    if data, err := os.ReadFile(kubeconfigPath); err == nil {
        existingContent = string(data)
    }
    
    // 生成新的标记块
    signalingSection := m.buildSignalingSection(clusters)
    
    // 合并配置
    mergedContent := m.replaceMarkedSection(existingContent, signalingSection)
    
    // 备份原文件
    if existingContent != "" {
        backupPath := kubeconfigPath + ".backup"
        os.WriteFile(backupPath, []byte(existingContent), 0600)
    }
    
    // 写入新配置
    return os.WriteFile(kubeconfigPath, []byte(mergedContent), 0600)
}

// buildSignalingSection 构建 Signaling 标记块
func (m *KubeconfigManager) buildSignalingSection(clusters []ClusterConfig) string {
    var sb strings.Builder
    
    sb.WriteString(m.markerBegin + "\n")
    sb.WriteString("apiVersion: v1\n")
    sb.WriteString("kind: Config\n")
    
    // Clusters
    sb.WriteString("clusters:\n")
    for _, c := range clusters {
        sb.WriteString(fmt.Sprintf("- cluster:\n"))
        sb.WriteString(fmt.Sprintf("    server: %s\n", c.Server))
        sb.WriteString(fmt.Sprintf("    insecure-skip-tls-verify: true\n"))
        sb.WriteString(fmt.Sprintf("  name: %s\n", c.Name))
    }
    
    // Contexts
    sb.WriteString("contexts:\n")
    for _, c := range clusters {
        sb.WriteString(fmt.Sprintf("- context:\n"))
        sb.WriteString(fmt.Sprintf("    cluster: %s\n", c.Name))
        sb.WriteString(fmt.Sprintf("    user: signaling-user\n"))
        sb.WriteString(fmt.Sprintf("  name: %s\n", c.Name))
    }
    
    // Users
    sb.WriteString("users:\n")
    sb.WriteString("- name: signaling-user\n")
    sb.WriteString("  user: {}\n")
    
    sb.WriteString(m.markerEnd + "\n")
    
    return sb.String()
}

// replaceMarkedSection 替换标记块
func (m *KubeconfigManager) replaceMarkedSection(content, newSection string) string {
    beginIdx := strings.Index(content, m.markerBegin)
    endIdx := strings.Index(content, m.markerEnd)
    
    if beginIdx >= 0 && endIdx >= 0 {
        // 找到结束标记的行尾
        endIdx += len(m.markerEnd)
        if endIdx < len(content) && content[endIdx] == '\n' {
            endIdx++
        }
        
        // 替换标记块
        return content[:beginIdx] + newSection + content[endIdx:]
    }
    
    // 没有标记块，追加到末尾
    if content != "" && !strings.HasSuffix(content, "\n") {
        content += "\n"
    }
    return content + newSection
}

// RemoveSignalingSection 删除 Signaling 标记块
func (m *KubeconfigManager) RemoveSignalingSection() error {
    homeDir, err := os.UserHomeDir()
    if err != nil {
        return err
    }
    
    kubeconfigPath := filepath.Join(homeDir, ".kube", "config")
    
    data, err := os.ReadFile(kubeconfigPath)
    if err != nil {
        return err
    }
    
    content := string(data)
    beginIdx := strings.Index(content, m.markerBegin)
    endIdx := strings.Index(content, m.markerEnd)
    
    if beginIdx >= 0 && endIdx >= 0 {
        endIdx += len(m.markerEnd)
        if endIdx < len(content) && content[endIdx] == '\n' {
            endIdx++
        }
        
        newContent := content[:beginIdx] + content[endIdx:]
        return os.WriteFile(kubeconfigPath, []byte(newContent), 0600)
    }
    
    return nil
}
```

## 总结

推荐使用**方案 1（自动合并到 ~/.kube/config）**，因为它提供了最佳的用户体验：

1. ✅ 零配置，点击即用
2. ✅ 符合 kubectl 标准使用习惯
3. ✅ 使用标记块，安全可靠
4. ✅ 支持多集群统一管理
5. ✅ 可以随时清理

这种方式既保持了灵活性，又提供了良好的用户体验，是企业级应用的最佳选择。
