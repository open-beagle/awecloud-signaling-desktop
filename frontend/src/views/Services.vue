<template>
  <div class="resource-page">
    <div class="page-header">
      <div>
        <h1>Kubernetes SVC</h1>
        <p>当前账号可访问的 Kubernetes Service</p>
      </div>
      <div class="manual-actions">
        <span v-if="lastFetchedAt" class="fetched-at">上次获取：{{ lastFetchedAt }}</span>
        <el-button v-if="!tenantOptions.length" size="small" :loading="loading" @click="initialize">手动获取</el-button>
        <button v-else class="icon-btn" title="手动获取服务" :disabled="loading || !activeTenantID" @click="loadResources">
          <el-icon :class="{ 'is-loading': loading }"><Refresh /></el-icon>
        </button>
      </div>
    </div>

    <div class="toolbar">
      <el-select
        v-if="tenantOptions.length"
        v-model="activeTenantID"
        class="tenant-filter"
        placeholder="选择 Tenant"
        :disabled="loading"
        @change="switchTenant"
      >
        <el-option v-for="tenant in tenantOptions" :key="tenant.id" :label="tenant.name || tenant.id" :value="tenant.id" />
      </el-select>
      <el-input v-model="searchQuery" clearable placeholder="搜索域名、Namespace 或服务" class="search-input" />
      <span class="count">{{ filteredServices.length }} / {{ services.length }} 个服务</span>
    </div>

    <div v-if="error" class="error-state">
      <span>{{ error }}</span>
      <button @click="tenantOptions.length ? loadResources() : initialize()">重试</button>
    </div>

    <div v-if="!error && routeWarnings.length" class="route-warning">
      <span>{{ routeWarningText }}。目标服务端口不变，请为当前设备选择其他本地端口。</span>
      <el-tooltip content="更换本地端口" placement="top">
        <button class="icon-btn warning-action" type="button" aria-label="更换本地端口" :disabled="loading" @click="openPortDialog(routeWarnings[0])">
          <el-icon><Setting /></el-icon>
        </button>
      </el-tooltip>
    </div>

    <div v-if="!error && !lastFetchedAt" class="manual-empty">
      <strong>{{ loading ? '正在获取 Kubernetes 服务' : '当前账号暂无可访问服务' }}</strong>
      <span>{{ loading ? '首次进入时会自动获取，后续优先使用当前 Tenant 缓存。' : '可以使用右上角手动获取。' }}</span>
    </div>

    <el-table v-if="!error && lastFetchedAt" v-loading="loading" :data="filteredServices" stripe height="100%" empty-text="暂无可访问服务">
      <el-table-column label="服务" min-width="220">
        <template #default="{ row }">
          <div class="primary">{{ row.service_name || row.display_name || '未命名服务' }}</div>
          <div class="secondary">{{ row.domain || '-' }}</div>
        </template>
      </el-table-column>
      <el-table-column label="Namespace" min-width="150" prop="namespace" />
      <el-table-column label="端口" width="110">
        <template #default="{ row }">
          <div>{{ row.port || '-' }}/{{ row.protocol || 'TCP' }}</div>
          <div v-if="row.local_port && row.local_port !== row.port" class="secondary">本地 {{ row.local_port }}</div>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <el-tooltip :content="row.local_error || ''" :disabled="!row.local_error" placement="top">
            <span class="status"><i :class="isAvailable(row) ? 'online' : 'offline'"></i>{{ statusLabel(row) }}</span>
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column label="访问地址" min-width="260">
        <template #default="{ row }"><code>{{ address(row) || '-' }}</code></template>
      </el-table-column>
      <el-table-column label="操作" width="88" fixed="right" align="center">
        <template #default="{ row }">
          <el-tooltip v-if="row.local_error" content="更换本地端口" placement="top">
            <button class="icon-btn table-action" type="button" aria-label="更换本地端口" @click="openPortDialog(row)">
              <el-icon><Setting /></el-icon>
            </button>
          </el-tooltip>
          <button class="icon-btn table-action" title="复制访问地址" :disabled="!address(row)" @click="copyAddress(row)">
            <el-icon><CopyDocument /></el-icon>
          </button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="portDialogVisible" title="更换本地端口" width="460px" :close-on-click-modal="false">
      <p class="dialog-description">仅修改当前设备的监听端口，Kubernetes Service 目标端口保持不变。</p>
      <div class="port-summary">
        <div><span>服务</span><strong>{{ editingService?.service_name || editingService?.display_name }}</strong></div>
        <div><span>目标端口</span><strong>{{ editingService?.port }}/{{ editingService?.protocol || 'TCP' }}</strong></div>
      </div>
      <div class="port-field">
        <label>新本地端口</label>
        <el-input-number v-model="newLocalPort" :min="1" :max="65535" :controls="false" />
      </div>
      <div class="port-preview">
        保存后访问地址为 <code>{{ editingService?.domain }}:{{ newLocalPort }}</code>，流量仍转发到目标端口 {{ editingService?.port }}。
      </div>
      <template #footer>
        <el-button @click="portDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="savingPort" :disabled="!canSavePort" @click="saveLocalPort">保存并启用</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument, Refresh, Setting } from '@element-plus/icons-vue'
import { SetContainerServiceLocalPort } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/internal/app/app'
import { useResourceCatalog } from '../composables/useResourceCatalog'
import type { Resource } from '../composables/useResourceCatalog'

const {
  resources,
  tenantOptions,
  activeTenantID,
  loading,
  error,
  lastFetchedAt,
  loadResources,
  switchTenant,
  initialize
} = useResourceCatalog()
const searchQuery = ref('')
const portDialogVisible = ref(false)
const editingService = ref<Resource | null>(null)
const newLocalPort = ref(18090)
const savingPort = ref(false)

function readableError(cause: any, fallback: string) {
  if (typeof cause === 'string' && cause.trim()) return cause
  if (typeof cause?.message === 'string' && cause.message.trim()) return cause.message
  if (typeof cause?.cause?.message === 'string' && cause.cause.message.trim()) return cause.cause.message
  return fallback
}

const services = computed(() => resources.value.filter(resource => resource.type === 'container_service'))
const routeWarnings = computed(() => services.value.filter(resource => resource.local_error))
const routeWarningText = computed(() => {
  const first = routeWarnings.value[0]?.local_error || ''
  const suffix = routeWarnings.value.length > 1 ? `；另有 ${routeWarnings.value.length - 1} 个服务失败` : ''
  return `${first}${suffix}`
})
const filteredServices = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return services.value
  return services.value.filter(resource =>
    [resource.domain, resource.namespace, resource.service_name, resource.display_name]
      .some(value => value?.toLowerCase().includes(query))
  )
})

function isAvailable(resource: Resource) {
  if (resource.local_error) return false
  return !resource.state || resource.state === 'available' || resource.state === 'degraded'
}

function statusLabel(resource: Resource) {
  if (resource.local_error) return '本地不可用'
  if (!resource.state) return '可用'
  return ({ available: '可用', degraded: '降级', pending: '等待目标', stopped: '已停止', revoked: '已撤销' } as Record<string, string>)[resource.state] || resource.state
}

function address(resource: Resource) {
  if (resource.local_error) return ''
  if (!resource.domain) return ''
  const localPort = resource.local_port || resource.port
  return localPort ? `${resource.domain}:${localPort}` : resource.domain
}

const canSavePort = computed(() => {
  const port = Number(newLocalPort.value)
  return !!editingService.value?.resource_id && Number.isInteger(port) && port >= 1 && port <= 65535 && port !== editingService.value.port
})

function openPortDialog(resource: Resource) {
  editingService.value = resource
  const targetPort = resource.port || 0
  newLocalPort.value = resource.local_port && resource.local_port !== targetPort
    ? resource.local_port
    : Math.min(targetPort + 10000, 65535)
  portDialogVisible.value = true
}

async function saveLocalPort() {
  if (!editingService.value?.resource_id || !canSavePort.value) return
  savingPort.value = true
  try {
    resources.value = ((await SetContainerServiceLocalPort(editingService.value.resource_id, Number(newLocalPort.value))) || []).filter(Boolean) as Resource[]
    portDialogVisible.value = false
    ElMessage.success(`本地端口已改为 ${newLocalPort.value}，目标端口保持 ${editingService.value.port}`)
  } catch (cause: any) {
    ElMessage.error(readableError(cause, '本地端口修改失败'))
  } finally {
    savingPort.value = false
  }
}

async function copyAddress(resource: Resource) {
  const value = address(resource)
  if (!value) return
  await navigator.clipboard.writeText(value)
  ElMessage.success('访问地址已复制')
}

onMounted(initialize)

</script>

<style scoped src="../styles/resource-list.css"></style>
