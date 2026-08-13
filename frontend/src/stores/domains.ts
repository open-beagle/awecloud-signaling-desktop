import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { GetDomainList } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/internal/app/app'

// 域名记录类型
export interface DomainItem {
  domain: string              // 域名（如 beagle-242.beijing.beagle）
  type: string                // 类型：ssh / k8sapi / container_ssh / container_service
  status: string              // 状态：online / offline
  service_ports?: number[]
  ssh_users?: string[]        // SSH 用户列表（ssh 类型时）
  namespace?: string
  service_name?: string
  region: string              // 区域名称（从 domain 解析，如 beijing）
  display_name?: string
  resource_id?: string
}

export const useDomainsStore = defineStore('domains', () => {
  const domains = ref<DomainItem[]>([])
  const loading = ref(false)
  const lastFetchedAt = ref('')
  let refreshPromise: Promise<void> | null = null

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
    lastFetchedAt.value = new Date().toLocaleString()
  }

  // 设置加载状态
  function setLoading(value: boolean) {
    loading.value = value
  }

  // 从数据流更新域名
  function updateFromStream(newDomains: DomainItem[]) {
    setDomains(newDomains)
  }

  // 清空域名列表
  function clearDomains() {
    domains.value = []
    lastFetchedAt.value = ''
  }

  async function refreshDomains() {
    if (refreshPromise) return refreshPromise
    refreshPromise = (async () => {
      loading.value = true
      try {
        const fetched = ((await GetDomainList()) || []).filter(Boolean) as DomainItem[]
        setDomains(fetched)
      } finally {
        loading.value = false
        refreshPromise = null
      }
    })()
    return refreshPromise
  }

  return {
    domains,
    loading,
    lastFetchedAt,
    hostsDomains,
    k8sDomains,
    setDomains,
    setLoading,
    updateFromStream,
    clearDomains,
    refreshDomains
  }
})
