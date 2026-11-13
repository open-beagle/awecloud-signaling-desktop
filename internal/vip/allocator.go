// Package vip 提供 VIP（Virtual IP）地址分配功能
// 使用 127.1.0.0/16 地址段，为每个远程服务分配唯一的本地地址
package vip

import (
	"fmt"
	"hash/fnv"
	"log"
	"strings"
	"sync"
)

const (
	vipsPerSubnet  = 254 // 1..254，跳过 .0 与 .255
	maxVIPCapacity = 256 * vipsPerSubnet
)

// AllocateCallback VIP 分配后的回调函数类型
// macOS 平台用于在分配 VIP 后自动添加 loopback alias
type AllocateCallback func(vip string) error

// Allocator VIP 地址分配器
// 将域名确定性映射到 127.1.x.x 地址（冲突时按固定步长线性探测），保证重启后映射稳定
type Allocator struct {
	// 域名 → VIP 映射
	domainToVIP map[string]string
	// VIP → 域名 反向映射
	vipToDomain map[string]string

	// VIP 分配后的回调（macOS 用于添加 loopback alias）
	onAllocate AllocateCallback

	mu sync.RWMutex
}

// NewAllocator 创建 VIP 分配器
func NewAllocator() *Allocator {
	return &Allocator{
		domainToVIP: make(map[string]string),
		vipToDomain: make(map[string]string),
	}
}

func normalizeVIPDomain(domain string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
}

func deterministicVIPSlot(normalizedDomain string) int {
	key := normalizedDomain
	if key == "" {
		key = "default"
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum64() % uint64(maxVIPCapacity))
}

func vipFromSlot(slot int) string {
	slot = ((slot % maxVIPCapacity) + maxVIPCapacity) % maxVIPCapacity
	hi := slot / vipsPerSubnet
	lo := slot%vipsPerSubnet + 1
	return fmt.Sprintf("127.1.%d.%d", hi, lo)
}

// DeterministicVIP 计算规范化域名对应的首选确定性 VIP（127.1.<0..255>.<1..254>）
func DeterministicVIP(domain string) string {
	key := normalizeVIPDomain(domain)
	if key == "" {
		return ""
	}
	return vipFromSlot(deterministicVIPSlot(key))
}

// SetOnAllocate 设置 VIP 分配后的回调函数
// macOS 平台用于在分配 VIP 后自动添加 loopback alias
func (a *Allocator) SetOnAllocate(cb AllocateCallback) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onAllocate = cb
}

// Allocate 为域名分配确定性 VIP 地址
// 如果域名已分配，返回已有的 VIP；否则按域名 FNV-1a 哈希分配，冲突时线性探测
func (a *Allocator) Allocate(domain string) (string, error) {
	key := normalizeVIPDomain(domain)
	if key == "" {
		return "", fmt.Errorf("域名不能为空")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// 已分配，直接返回
	if vip, ok := a.domainToVIP[key]; ok {
		return vip, nil
	}

	if len(a.vipToDomain) >= maxVIPCapacity {
		return "", fmt.Errorf("VIP 地址耗尽（最多 %d 个）", maxVIPCapacity)
	}

	startSlot := deterministicVIPSlot(key)
	var selected string
	for step := 0; step < maxVIPCapacity; step++ {
		candidate := vipFromSlot((startSlot + step) % maxVIPCapacity)
		if owner, taken := a.vipToDomain[candidate]; !taken || owner == key {
			selected = candidate
			break
		}
	}
	if selected == "" {
		return "", fmt.Errorf("VIP 地址耗尽（最多 %d 个）", maxVIPCapacity)
	}

	a.domainToVIP[key] = selected
	a.vipToDomain[selected] = key

	// 触发回调（macOS 用于添加 loopback alias）
	if a.onAllocate != nil {
		if err := a.onAllocate(selected); err != nil {
			log.Printf("[VIP] 分配回调失败 (%s -> %s): %v", key, selected, err)
		}
	}

	return selected, nil
}

// Resolve 根据 VIP 查找域名
func (a *Allocator) Resolve(vip string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	domain, ok := a.vipToDomain[strings.TrimSpace(vip)]
	return domain, ok
}

// GetVIP 根据域名查找 VIP（不分配）
func (a *Allocator) GetVIP(domain string) (string, bool) {
	key := normalizeVIPDomain(domain)
	a.mu.RLock()
	defer a.mu.RUnlock()
	vip, ok := a.domainToVIP[key]
	return vip, ok
}

// Release removes a stale DNS mapping. Platform loopback aliases may remain
// until Cleanup, but without this mapping the local DNS server returns NXDOMAIN.
func (a *Allocator) Release(domain string) {
	key := normalizeVIPDomain(domain)
	a.mu.Lock()
	defer a.mu.Unlock()
	vip, ok := a.domainToVIP[key]
	if !ok {
		return
	}
	delete(a.domainToVIP, key)
	delete(a.vipToDomain, vip)
}

// GetAll 获取所有映射（用于调试/展示）
func (a *Allocator) GetAll() map[string]string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make(map[string]string, len(a.domainToVIP))
	for k, v := range a.domainToVIP {
		result[k] = v
	}
	return result
}

// Count 获取已分配的 VIP 数量
func (a *Allocator) Count() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.domainToVIP)
}
