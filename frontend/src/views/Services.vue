<template>
  <div class="resource-page">
    <div class="page-header">
      <div>
        <h1>SVC</h1>
        <p>当前账号可访问的 Kubernetes Service</p>
      </div>
      <button class="icon-btn" title="刷新服务" :disabled="loading" @click="loadResources">
        <el-icon :class="{ 'is-loading': loading }"><Refresh /></el-icon>
      </button>
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
      <button @click="loadTenants">重试</button>
    </div>

    <el-table v-else v-loading="loading" :data="filteredServices" stripe height="100%" empty-text="暂无可访问服务">
      <el-table-column label="服务" min-width="220">
        <template #default="{ row }">
          <div class="primary">{{ row.service_name || row.display_name || '未命名服务' }}</div>
          <div class="secondary">{{ row.domain || '-' }}</div>
        </template>
      </el-table-column>
      <el-table-column label="Namespace" min-width="150" prop="namespace" />
      <el-table-column label="端口" width="110">
        <template #default="{ row }">{{ row.port || '-' }}</template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <span class="status"><i :class="isAvailable(row) ? 'online' : 'offline'"></i>{{ statusLabel(row) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="访问地址" min-width="260">
        <template #default="{ row }"><code>{{ address(row) || '-' }}</code></template>
      </el-table-column>
      <el-table-column label="操作" width="72" fixed="right" align="center">
        <template #default="{ row }">
          <button class="icon-btn table-action" title="复制访问地址" :disabled="!address(row)" @click="copyAddress(row)">
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
import { useResourceCatalog } from '../composables/useResourceCatalog'
import type { Resource } from '../composables/useResourceCatalog'

const {
  resources,
  tenantOptions,
  activeTenantID,
  loading,
  error,
  loadResources,
  loadTenants,
  switchTenant
} = useResourceCatalog()
const searchQuery = ref('')
let refreshTimer: number | null = null

const services = computed(() => resources.value.filter(resource => resource.type === 'container_service'))
const filteredServices = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return services.value
  return services.value.filter(resource =>
    [resource.domain, resource.namespace, resource.service_name, resource.display_name]
      .some(value => value?.toLowerCase().includes(query))
  )
})

function isAvailable(resource: Resource) {
  return !resource.state || resource.state === 'available' || resource.state === 'degraded'
}

function statusLabel(resource: Resource) {
  if (!resource.state) return '可用'
  return ({ available: '可用', degraded: '降级', pending: '等待目标', stopped: '已停止', revoked: '已撤销' } as Record<string, string>)[resource.state] || resource.state
}

function address(resource: Resource) {
  if (!resource.domain) return ''
  return resource.port ? `${resource.domain}:${resource.port}` : resource.domain
}

async function copyAddress(resource: Resource) {
  const value = address(resource)
  if (!value) return
  await navigator.clipboard.writeText(value)
  ElMessage.success('访问地址已复制')
}

onMounted(() => {
  loadTenants()
  refreshTimer = window.setInterval(loadResources, 30000)
})

onUnmounted(() => {
  if (refreshTimer) window.clearInterval(refreshTimer)
})
</script>

<style scoped src="../styles/resource-list.css"></style>
