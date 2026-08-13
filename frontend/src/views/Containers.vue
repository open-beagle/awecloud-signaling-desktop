<template>
  <div class="resource-page">
    <div class="page-header">
      <div>
        <h1>Kubernetes Pod</h1>
        <p>当前 Tenant 授权的 Pod 容器</p>
      </div>
      <div class="manual-actions">
        <span v-if="lastFetchedAt" class="fetched-at">上次获取：{{ lastFetchedAt }}</span>
        <el-button v-if="!tenantOptions.length" size="small" :loading="loading" @click="loadTenants">获取 Tenant</el-button>
        <button v-else class="icon-btn" title="手动刷新 Pod" :disabled="loading || !activeTenantID" @click="loadResources">
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
      <el-input v-model="searchQuery" clearable placeholder="搜索名称或域名" class="search-input" />
		<span class="count">{{ filteredContainers.length }} / {{ containers.length }} 个 Pod</span>
    </div>

    <div v-if="error" class="error-state">
      <span>{{ error }}</span>
      <button @click="tenantOptions.length ? loadResources() : loadTenants()">重试</button>
    </div>

    <div v-else-if="!lastFetchedAt" class="manual-empty">
      <strong>{{ tenantOptions.length ? '尚未获取当前 Tenant 的资源' : '尚未获取 Tenant' }}</strong>
      <span>Desktop 不会自动拉取数据，请由管理员手动获取。</span>
      <el-button type="primary" :loading="loading" @click="tenantOptions.length ? loadResources() : loadTenants()">
        {{ tenantOptions.length ? '获取资源' : '获取 Tenant' }}
      </el-button>
    </div>

	<el-table v-else v-loading="loading" :data="filteredContainers" stripe height="100%" empty-text="暂无可访问 Pod">
      <el-table-column label="Pod" min-width="220">
        <template #default="{ row }">
          <div class="primary">{{ row.pod_name || '未命名 Pod' }}</div>
          <div class="secondary">{{ workloadLabel(row) }}</div>
        </template>
      </el-table-column>
      <el-table-column label="Container" min-width="170">
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
import { computed, ref } from 'vue'
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
  lastFetchedAt,
  loadResources,
  loadTenants,
  switchTenant
} = useResourceCatalog()
const searchQuery = ref('')

const containers = computed(() => resources.value.filter(resource => resource.type === 'container_ssh'))
const filteredContainers = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  return containers.value.filter(resource => {
    if (!query) return true
    return [resource.pod_name, resource.container_name, resource.workload_name, resource.namespace, resource.domain]
      .some(value => value?.toLowerCase().includes(query))
  })
})

function workloadLabel(resource: Resource) {
  const workload = [resource.workload_kind, resource.workload_name].filter(Boolean).join('/')
  return [resource.namespace, workload].filter(Boolean).join(' · ') || '-'
}

function targetLabel(resource: Resource) {
  return resource.container_name || '-'
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

</script>

<style scoped src="../styles/resource-list.css"></style>
