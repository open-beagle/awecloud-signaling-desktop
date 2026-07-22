<template>
  <div class="resources-page">
    <div class="page-header">
      <div>
        <h1>资源浏览</h1>
        <p>当前账号可访问的主机、容器、K8S 集群和服务</p>
      </div>
      <button class="icon-btn" title="刷新资源" :disabled="loading" @click="loadResources">
        <el-icon :class="{ 'is-loading': loading }"><Refresh /></el-icon>
      </button>
    </div>

    <div class="toolbar">
      <el-input v-model="searchQuery" clearable placeholder="搜索名称、域名或 Tenant" class="search-input" />
      <el-select v-model="typeFilter" class="type-filter">
        <el-option label="全部类型" value="all" />
        <el-option label="SSH 主机" value="ssh" />
        <el-option label="ContainerSSH" value="container_ssh" />
        <el-option label="K8S API" value="k8sapi" />
        <el-option label="K8S 服务" value="k8ssvc" />
      </el-select>
      <span class="count">{{ filteredResources.length }} / {{ resources.length }} 个资源</span>
    </div>

    <div v-if="error" class="error-state">
      <span>{{ error }}</span>
      <button @click="loadResources">重试</button>
    </div>

    <el-table v-else v-loading="loading" :data="filteredResources" stripe height="100%" empty-text="暂无可访问资源">
      <el-table-column label="资源" min-width="230">
        <template #default="{ row }">
          <div class="resource-name">{{ displayName(row) }}</div>
          <div class="resource-domain">{{ row.domain || '-' }}</div>
        </template>
      </el-table-column>
      <el-table-column label="类型" width="130">
        <template #default="{ row }">
          <el-tag size="small" :type="typeTag(row.type)">{{ typeLabel(row.type) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="范围" min-width="170">
        <template #default="{ row }">
          <span>{{ scopeLabel(row) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <span class="status"><i :class="isAvailable(row) ? 'online' : 'offline'"></i>{{ statusLabel(row) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="连接信息" min-width="260">
        <template #default="{ row }">
          <code>{{ connectionText(row) }}</code>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="72" fixed="right" align="center">
        <template #default="{ row }">
          <button
            class="icon-btn table-action"
            title="复制连接信息"
            :disabled="!connectionText(row)"
            @click="copyConnection(row)"
          >
            <el-icon><CopyDocument /></el-icon>
          </button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument, Refresh } from '@element-plus/icons-vue'
import { GetResources } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/app'

interface Resource {
  type: string
  agent_name?: string
  domain?: string
  ssh_users?: string[]
  namespaces?: string[]
  namespace?: string
  service_name?: string
  port?: number
  display_name?: string
  tenant_name?: string
  state?: string
  target_revision?: number
  ssh_user?: string
}

const resources = ref<Resource[]>([])
const loading = ref(false)
const error = ref('')
const searchQuery = ref('')
const typeFilter = ref('all')
let refreshTimer: number | null = null

const filteredResources = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return resources.value.filter(resource => {
    if (typeFilter.value !== 'all' && resource.type !== typeFilter.value) return false
    if (!query) return true
    return [displayName(resource), resource.domain, resource.tenant_name, resource.namespace, resource.service_name]
      .some(value => value?.toLowerCase().includes(query))
  })
})

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

function displayName(resource: Resource) {
  return resource.display_name || resource.service_name || resource.agent_name || resource.domain || '未命名资源'
}

function typeLabel(type: string) {
  return ({ ssh: 'SSH 主机', container_ssh: 'ContainerSSH', k8sapi: 'K8S API', k8ssvc: 'K8S 服务' } as Record<string, string>)[type] || type
}

function typeTag(type: string) {
  return ({ container_ssh: 'success', k8sapi: 'warning', k8ssvc: 'info' } as Record<string, string>)[type] || ''
}

function scopeLabel(resource: Resource) {
  if (resource.type === 'container_ssh') {
    return [resource.tenant_name, resource.target_revision ? `revision ${resource.target_revision}` : ''].filter(Boolean).join(' / ') || '-'
  }
  if (resource.type === 'k8ssvc') return resource.namespace || '-'
  if (resource.type === 'k8sapi') return resource.namespaces?.length ? resource.namespaces.join(', ') : '全部 Namespace'
  return resource.agent_name || '-'
}

function isAvailable(resource: Resource) {
  return !resource.state || resource.state === 'available' || resource.state === 'degraded'
}

function statusLabel(resource: Resource) {
  if (!resource.state) return '可用'
  return ({ available: '可用', degraded: '降级', pending: '等待目标', stopped: '已停止', revoked: '已撤销' } as Record<string, string>)[resource.state] || resource.state
}

function connectionText(resource: Resource) {
  if (!resource.domain) return ''
  if (resource.type === 'container_ssh') return `ssh ${resource.ssh_user || 'container'}@${resource.domain}`
  if (resource.type === 'ssh') return resource.ssh_users?.length ? `ssh ${resource.ssh_users[0]}@${resource.domain}` : resource.domain
  if (resource.type === 'k8sapi') return `${resource.domain}:6443`
  if (resource.type === 'k8ssvc') return `${resource.domain}:${resource.port || ''}`
  return resource.domain
}

async function copyConnection(resource: Resource) {
  const text = connectionText(resource)
  if (!text) return
  await navigator.clipboard.writeText(text)
  ElMessage.success('连接信息已复制')
}

onMounted(() => {
  loadResources()
  refreshTimer = window.setInterval(loadResources, 30000)
})

onUnmounted(() => {
  if (refreshTimer) window.clearInterval(refreshTimer)
})
</script>

<style scoped>
.resources-page {
  height: 100%;
  min-height: 0;
  padding: 18px 20px 20px;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
}

.page-header h1 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: #262626;
}

.page-header p {
  margin: 3px 0 0;
  color: #8c8c8c;
  font-size: 12px;
}

.toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
}

.search-input { width: 280px; }
.type-filter { width: 150px; }
.count { margin-left: auto; color: #8c8c8c; font-size: 12px; }

.resource-name { color: #262626; font-size: 13px; font-weight: 500; }
.resource-domain { margin-top: 2px; color: #8c8c8c; font-size: 12px; }

.status { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; }
.status i { width: 7px; height: 7px; border-radius: 50%; background: #bfbfbf; }
.status i.online { background: #52c41a; }
.status i.offline { background: #bfbfbf; }

code { color: #595959; font-size: 12px; white-space: normal; word-break: break-all; }

.icon-btn {
  width: 32px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid #d9d9d9;
  border-radius: 4px;
  background: #fff;
  color: #595959;
  cursor: pointer;
}

.icon-btn:hover:not(:disabled) { color: #1677ff; border-color: #1677ff; }
.icon-btn:disabled { color: #bfbfbf; cursor: not-allowed; }
.table-action { width: 28px; height: 28px; }

.error-state {
  padding: 48px 0;
  text-align: center;
  color: #cf1322;
}

.error-state button {
  margin-left: 12px;
  border: 0;
  background: transparent;
  color: #1677ff;
  cursor: pointer;
}
</style>
