import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

// 域名记录类型
export interface DomainItem {
  domain: string              // 域名（如 beagle-242.beijing.beagle）
  type: string                // 类型：ssh / k8sapi / k8ssvc
  status: string              // 状态：online / offline
  service_ports?: number[]    // K8S Service 端口列表（k8ssvc 类型时）
  ssh_users?: string[]        // SSH 用户列表（ssh 类型时）
  namespace?: string          // K8S 命名空间（k8ssvc 类型时）
  service_name?: string       // K8S Service 名称（k8ssvc 类型时）
  region: string              // 区域名称（从 domain 解析，如 beijing）
}

export const useDomainsStore = defineStore('domains', () => {
  const domains = ref<DomainItem[]>([])
  const loading = ref(false)

  // 计算属性：K8S Service 域名列表（我的服务）
  const servicesDomains = computed(() => {
    return domains.value.filter(d => d.type === 'k8ssvc')
  })

  // 计算属性：SSH 域名列表（我的主机）
  // 按 domain 聚合，因为一个主机可能有多个用户
  const hostsDomains = computed(() => {
    const sshDomains = domains.value.filter(d => d.type === 'ssh')
    // 已经按 domain 聚合了，直接返回
    return sshDomains
  })

  // 计算属性：K8S API 域名列表（我的K8S）
  const k8sDomains = computed(() => {
    return domains.value.filter(d => d.type === 'k8sapi')
  })

  // 设置域名列表
  function setDomains(newDomains: DomainItem[]) {
    domains.value = newDomains
  }

  // 设置加载状态
  function setLoading(value: boolean) {
    loading.value = value
  }

  // 从数据流更新域名
  function updateFromStream(newDomains: DomainItem[]) {
    domains.value = newDomains
  }

  // 清空域名列表
  function clearDomains() {
    domains.value = []
  }

  return {
    domains,
    loading,
    servicesDomains,
    hostsDomains,
    k8sDomains,
    setDomains,
    setLoading,
    updateFromStream,
    clearDomains
  }
})
