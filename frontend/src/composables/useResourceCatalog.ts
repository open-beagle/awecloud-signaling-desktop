import { ref } from 'vue'
import { GetResources, GetResourceTenants, SwitchResourceTenant } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/internal/app/app'

export interface Resource {
  type: string
  agent_name?: string
  domain?: string
  ssh_users?: string[]
  namespaces?: string[]
  namespace?: string
  service_name?: string
  port?: number
  display_name?: string
  tenant_id?: string
  tenant_name?: string
  state?: string
  local_error?: string
  local_port?: number
  resource_id?: string
  target_revision?: number
  port_name?: string
  protocol?: string
  workload_kind?: string
  workload_name?: string
  pod_uid?: string
  pod_name?: string
  container_name?: string
}

interface ResourceTenant {
  id: string
  name: string
}

const resources = ref<Resource[]>([])
const tenantOptions = ref<ResourceTenant[]>([])
const activeTenantID = ref('')
const loading = ref(false)
const error = ref('')
const lastFetchedAt = ref('')
let inFlight: Promise<void> | null = null

function readableError(cause: any, fallback: string) {
  if (typeof cause === 'string' && cause.trim()) return cause
  if (typeof cause?.message === 'string' && cause.message.trim()) return cause.message
  if (typeof cause?.cause?.message === 'string' && cause.cause.message.trim()) return cause.cause.message
  return fallback
}

export function useResourceCatalog() {
  async function loadResources() {
    if (!activeTenantID.value) {
      error.value = '请先手动获取并选择 Tenant'
      return
    }
    if (inFlight) return inFlight
    inFlight = (async () => {
      loading.value = true
      error.value = ''
      try {
        const fetched = ((await GetResources()) || []).filter(Boolean) as Resource[]
        resources.value = fetched
        const fetchedAt = new Date().toLocaleString()
        lastFetchedAt.value = fetchedAt
      } catch (cause: any) {
        error.value = readableError(cause, '资源加载失败，请检查网络后重试')
      } finally {
        loading.value = false
        inFlight = null
      }
    })()
    return inFlight
  }

  async function loadTenants() {
    if (inFlight) return inFlight
    inFlight = (async () => {
      loading.value = true
      error.value = ''
      try {
        tenantOptions.value = ((await GetResourceTenants()) || []) as ResourceTenant[]
        if (!tenantOptions.value.some(tenant => tenant.id === activeTenantID.value)) {
          activeTenantID.value = tenantOptions.value[0]?.id || ''
        }
        if (activeTenantID.value) await selectTenantInternal()
      } catch (cause: any) {
        error.value = readableError(cause, 'Tenant 加载失败，请检查网络后重试')
      } finally {
        loading.value = false
        inFlight = null
      }
    })()
    return inFlight
  }

  async function selectTenantInternal() {
    await SwitchResourceTenant(activeTenantID.value)
    resources.value = []
    lastFetchedAt.value = ''
  }

  async function switchTenant() {
    if (!activeTenantID.value || inFlight) return
    loading.value = true
    error.value = ''
    try {
      await selectTenantInternal()
    } catch (cause: any) {
      resources.value = []
      lastFetchedAt.value = ''
      error.value = readableError(cause, 'Tenant 切换失败，请重试')
    } finally {
      loading.value = false
    }
  }

  return {
    resources,
    tenantOptions,
    activeTenantID,
    loading,
    error,
    lastFetchedAt,
    loadResources,
    loadTenants,
    switchTenant
  }
}
