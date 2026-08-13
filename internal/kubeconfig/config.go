package kubeconfig

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	UserName  = "who"
	UserToken = "whoisyourdaddy"
)

type Cluster struct {
	Name   string
	Server string
}

type Config struct {
	APIVersion     string                 `yaml:"apiVersion"`
	Clusters       []NamedCluster         `yaml:"clusters"`
	Contexts       []NamedContext         `yaml:"contexts"`
	CurrentContext string                 `yaml:"current-context"`
	Kind           string                 `yaml:"kind"`
	Preferences    map[string]interface{} `yaml:"preferences"`
	Users          []NamedUser            `yaml:"users"`
	Extra          map[string]interface{} `yaml:",inline"`
}

type NamedCluster struct {
	Cluster map[string]interface{} `yaml:"cluster"`
	Name    string                 `yaml:"name"`
	Extra   map[string]interface{} `yaml:",inline"`
}

type NamedContext struct {
	Context map[string]interface{} `yaml:"context"`
	Name    string                 `yaml:"name"`
	Extra   map[string]interface{} `yaml:",inline"`
}

type NamedUser struct {
	Name  string                 `yaml:"name"`
	User  map[string]interface{} `yaml:"user"`
	Extra map[string]interface{} `yaml:",inline"`
}

// Merge updates only matching Beagle entries and preserves unrelated kubeconfig data.
func Merge(existing []byte, clusters []Cluster, currentContext string) ([]byte, error) {
	if len(clusters) == 0 {
		return nil, fmt.Errorf("没有可安装的 Kubernetes 集群")
	}

	cfg := Config{}
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := yaml.Unmarshal(existing, &cfg); err != nil {
			return nil, fmt.Errorf("现有 kubeconfig 格式无效: %w", err)
		}
	}
	if strings.TrimSpace(cfg.APIVersion) == "" {
		cfg.APIVersion = "v1"
	}
	if strings.TrimSpace(cfg.Kind) == "" {
		cfg.Kind = "Config"
	}
	if cfg.Preferences == nil {
		cfg.Preferences = map[string]interface{}{}
	}

	for _, cluster := range clusters {
		name := strings.TrimSpace(cluster.Name)
		server := strings.TrimSpace(cluster.Server)
		if name == "" || server == "" {
			return nil, fmt.Errorf("集群名称和服务器地址不能为空")
		}
		cfg.Clusters = replaceCluster(cfg.Clusters, NamedCluster{
			Name: name,
			Cluster: map[string]interface{}{
				"insecure-skip-tls-verify": true,
				"server":                   server,
			},
		})
		cfg.Contexts = replaceContext(cfg.Contexts, NamedContext{
			Name: name,
			Context: map[string]interface{}{
				"cluster": name,
				"user":    UserName,
			},
		})
	}
	cfg.Users = replaceUser(cfg.Users, NamedUser{
		Name: UserName,
		User: map[string]interface{}{"token": UserToken},
	})
	cfg.CurrentContext = currentContext

	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&cfg); err != nil {
		return nil, fmt.Errorf("生成 kubeconfig: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("完成 kubeconfig: %w", err)
	}
	return output.Bytes(), nil
}

func replaceCluster(items []NamedCluster, replacement NamedCluster) []NamedCluster {
	result := make([]NamedCluster, 0, len(items)+1)
	for _, item := range items {
		if item.Name != replacement.Name {
			result = append(result, item)
		}
	}
	return append(result, replacement)
}

func replaceContext(items []NamedContext, replacement NamedContext) []NamedContext {
	result := make([]NamedContext, 0, len(items)+1)
	for _, item := range items {
		if item.Name != replacement.Name {
			result = append(result, item)
		}
	}
	return append(result, replacement)
}

func replaceUser(items []NamedUser, replacement NamedUser) []NamedUser {
	result := make([]NamedUser, 0, len(items)+1)
	for _, item := range items {
		if item.Name != replacement.Name {
			result = append(result, item)
		}
	}
	return append(result, replacement)
}
