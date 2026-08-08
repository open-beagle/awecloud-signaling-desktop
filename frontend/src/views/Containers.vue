<template>
  <div class="resource-page">
    <div class="page-header">
      <div>
        <h1>Pods</h1>
        <p>当前 Tenant 授权的容器终端</p>
      </div>
      <button class="icon-btn" title="刷新容器资源" :disabled="loading" @click="loadResources">
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
      <el-input v-model="searchQuery" clearable placeholder="搜索名称或域名" class="search-input" />
      <span class="count">{{ filteredContainers.length }} / {{ containers.length }} 个资源</span>
    </div>

    <div v-if="error" class="error-state">
      <span>{{ error }}</span>
      <button @click="loadTenants">重试</button>
    </div>

    <el-table v-else v-loading="loading" :data="filteredContainers" stripe height="100%" empty-text="暂无可访问容器资源">
      <el-table-column label="资源" min-width="220">
        <template #default="{ row }">
          <div class="primary">{{ row.display_name || row.service_name || row.domain || '未命名资源' }}</div>
          <div class="secondary">{{ row.domain || '-' }}</div>
        </template>
      </el-table-column>
      <el-table-column label="目标" min-width="170">
        <template #default="{ row }">{{ targetLabel(row) }}</template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <span class="status"><i :class="isAvailable(row) ? 'online' : 'offline'"></i>{{ statusLabel(row) }}</span>
        </template>
      </el-table-column>
      <el-table-column label="连接信息" min-width="260">
        <template #default="{ row }"><code>{{ connectionText(row) || '-' }}</code></template>
      </el-table-column>
      <el-table-column label="操作" width="72" fixed="right" align="center">
        <template #default="{ row }">
          <button class="icon-btn table-action" title="复制连接信息" :disabled="!connectionText(row)" @click="copyConnection(row)">
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

const containers = computed(() => resources.value.filter(resource => resource.type === 'container_ssh'))
const filteredContainers = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return containers.value.filter(resource => {
    if (!query) return true
    return [resource.display_name, resource.domain, resource.tenant_name]
      .some(value => value?.toLowerCase().includes(query))
  })
})

function targetLabel(resource: Resource) {
  return resource.target_revision ? `Revision ${resource.target_revision}` : '-'
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
  return resource.ssh_users?.length ? `ssh ${resource.ssh_users[0]}@${resource.domain}` : resource.domain
}

async function copyConnection(resource: Resource) {
  const value = connectionText(resource)
  if (!value) return
  await navigator.clipboard.writeText(value)
  ElMessage.success('连接信息已复制')
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
