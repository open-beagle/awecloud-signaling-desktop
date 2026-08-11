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

export function useResourceCatalog() {
  const resources = ref<Resource[]>([])
  const tenantOptions = ref<ResourceTenant[]>([])
  const activeTenantID = ref('')
  const loading = ref(false)
  const error = ref('')

  async function loadResources() {
    loading.value = true
    error.value = ''
    try {
      resources.value = ((await GetResources()) || []).filter(Boolean) as Resource[]
    } catch (cause: any) {
      error.value = cause?.message || '资源加载失败'
    } finally {
      loading.value = false
    }
  }

  async function loadTenants() {
    loading.value = true
    error.value = ''
    try {
      tenantOptions.value = ((await GetResourceTenants()) || []) as ResourceTenant[]
      if (!tenantOptions.value.length) {
        resources.value = ((await GetResources()) || []).filter(Boolean) as Resource[]
        return
      }
      if (!tenantOptions.value.some(tenant => tenant.id === activeTenantID.value)) {
        activeTenantID.value = tenantOptions.value[0].id
      }
      resources.value = ((await SwitchResourceTenant(activeTenantID.value)) || []).filter(Boolean) as Resource[]
    } catch (cause: any) {
      resources.value = []
      error.value = cause?.message || 'Tenant 资源加载失败'
    } finally {
      loading.value = false
    }
  }

  async function switchTenant() {
    if (!activeTenantID.value) return
    loading.value = true
    error.value = ''
    resources.value = []
    try {
      resources.value = ((await SwitchResourceTenant(activeTenantID.value)) || []).filter(Boolean) as Resource[]
    } catch (cause: any) {
      error.value = cause?.message || 'Tenant 切换失败'
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
    loadResources,
    loadTenants,
    switchTenant
  }
}
