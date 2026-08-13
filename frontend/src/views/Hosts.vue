<template>
  <div class="resource-page">
    <div class="page-header">
      <div>
        <h1>SSH</h1>
        <p>当前账号可访问的主机终端</p>
      </div>
      <div class="manual-actions">
        <span v-if="domainsStore.lastFetchedAt" class="fetched-at">上次获取：{{ domainsStore.lastFetchedAt }}</span>
        <button class="icon-btn" title="更新 SSH 列表" :disabled="domainsStore.loading" @click="refreshDomains">
          <el-icon :class="{ 'is-loading': domainsStore.loading }"><Refresh /></el-icon>
        </button>
      </div>
    </div>

    <div class="toolbar">
      <el-input v-model="searchQuery" clearable placeholder="搜索主机、地址或用户" class="search-input" />
      <span class="count">{{ filteredDomains.length }} / {{ hostsDomains.length }} 台主机</span>
    </div>

    <el-table :data="filteredDomains" stripe height="100%" empty-text="暂无可访问主机">
      <el-table-column label="主机" min-width="220">
        <template #default="{ row }">
          <div class="primary">{{ row.display_name || row.domain }}</div>
          <div class="secondary">{{ row.domain }}</div>
        </template>
      </el-table-column>
      <el-table-column label="区域" min-width="130">
        <template #default="{ row }">{{ row.region || '-' }}</template>
      </el-table-column>
      <el-table-column label="用户" min-width="220">
        <template #default="{ row }">
          <template v-if="row.ssh_users?.length">
            <div class="primary">{{ row.ssh_users.length === 1 ? row.ssh_users[0] : `${row.ssh_users.length} 个用户` }}</div>
            <div v-if="row.ssh_users.length > 1" class="secondary user-summary">{{ row.ssh_users.join(', ') }}</div>
          </template>
          <span v-else class="secondary">暂无授权用户</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <span class="status"><i :class="row.status === 'offline' ? 'offline' : 'online'"></i>{{ row.status === 'offline' ? '离线' : '可用' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="84" fixed="right" align="center">
        <template #default="{ row }">
          <el-tooltip :content="row.ssh_users?.length > 1 ? '选择用户并复制' : '复制 SSH 命令'" placement="top">
            <button
              class="icon-btn table-action"
              type="button"
              :aria-label="row.ssh_users?.length > 1 ? '选择 SSH 用户并复制' : '复制 SSH 命令'"
              :disabled="!row.ssh_users?.length"
              @click="handleCopy(row)"
            >
              <el-icon><component :is="row.ssh_users?.length > 1 ? UserFilled : CopyDocument" /></el-icon>
            </button>
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="userDialogVisible" title="选择 SSH 用户" width="600px" :close-on-click-modal="false">
      <p class="dialog-description">{{ selectedHost?.display_name || selectedHost?.domain }} 支持多个登录用户，请选择需要的身份。</p>
      <div class="ssh-user-list">
        <div v-for="user in selectedHost?.ssh_users || []" :key="user" class="ssh-user-row">
          <strong>{{ user }}</strong>
          <code>{{ sshCommand(selectedHost, user) }}</code>
          <el-button size="small" :icon="CopyDocument" @click="copyCommand(selectedHost, user)">复制</el-button>
        </div>
      </div>
      <div v-if="selectedHost?.ssh_users?.includes('root')" class="admin-note">管理员账号不会默认选择，请确认当前操作需要对应权限。</div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument, Refresh, UserFilled } from '@element-plus/icons-vue'
import { useDomainsStore } from '../stores/domains'
import type { DomainItem } from '../stores/domains'

const domainsStore = useDomainsStore()
const hostsDomains = computed(() => domainsStore.hostsDomains)
const searchQuery = ref('')
const userDialogVisible = ref(false)
const selectedHost = ref<DomainItem | null>(null)

async function refreshDomains() {
  try {
    await domainsStore.refreshDomains()
  } catch (cause: any) {
    ElMessage.error(cause?.message || 'SSH 列表更新失败')
  }
}

const filteredDomains = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return hostsDomains.value
  return hostsDomains.value.filter(domain =>
    [domain.domain, domain.display_name, domain.region, ...(domain.ssh_users || [])]
      .some(value => value?.toLowerCase().includes(query))
  )
})

function sshCommand(domain: DomainItem | null, user: string) {
  return domain ? `ssh ${user}@${domain.domain}` : ''
}

async function copyCommand(domain: DomainItem | null, user: string) {
  if (!domain) return
  const command = sshCommand(domain, user)
  await navigator.clipboard.writeText(command)
  userDialogVisible.value = false
  ElMessage.success(`已复制：${command}`)
}

function handleCopy(domain: DomainItem) {
  const users = domain.ssh_users || []
  if (users.length === 1) {
    copyCommand(domain, users[0])
    return
  }
  if (users.length > 1) {
    selectedHost.value = domain
    userDialogVisible.value = true
  }
}
</script>

<style scoped src="../styles/resource-list.css"></style>
<style scoped>
.user-summary { max-width: 320px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ssh-user-list { border: 1px solid #e4e7ed; border-radius: 6px; }
.ssh-user-row { min-height: 58px; display: grid; grid-template-columns: 110px minmax(0, 1fr) auto; align-items: center; gap: 14px; padding: 8px 12px; border-bottom: 1px solid #ebeef5; }
.ssh-user-row:last-child { border-bottom: 0; }
.ssh-user-row strong { color: #303133; font-size: 13px; }
.ssh-user-row code { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.admin-note { margin-top: 12px; padding: 9px 11px; color: #8a5b27; background: #fff7e8; border: 1px solid #f0c67e; border-radius: 6px; font-size: 12px; }
</style>
