<template>
  <Layout>
    <div class="hosts-page">
      <!-- 页面头部 -->
      <div class="page-header">
        <div class="header-left">
          <h2>我的主机</h2>
          <el-tag v-if="hosts.length > 0">
            共 {{ hosts.length }} 台主机
          </el-tag>
        </div>
        <div class="header-right">
          <!-- 搜索框（可展开/收起） -->
          <transition name="search-expand">
            <el-input
              v-if="searchExpanded"
              ref="searchInputRef"
              v-model="searchQuery"
              placeholder="搜索主机"
              :prefix-icon="Search"
              clearable
              class="search-input"
              @blur="handleSearchBlur"
            />
          </transition>
          
          <el-tooltip :content="searchExpanded ? '关闭搜索' : '搜索'" placement="bottom">
            <el-button 
              :icon="Search" 
              @click="toggleSearch"
              circle
            />
          </el-tooltip>
          
          <el-tooltip content="刷新" placement="bottom">
            <el-button 
              :icon="Refresh" 
              @click="handleRefresh" 
              :loading="loading"
              circle
            />
          </el-tooltip>
        </div>
      </div>

      <!-- 主机列表 -->
      <div class="hosts-content">
        <el-empty 
          v-if="!loading && hosts.length === 0" 
          description="暂无可用主机" 
        />
        
        <el-empty 
          v-else-if="!loading && filteredHosts.length === 0" 
          description="没有找到匹配的主机" 
        />
        
        <div v-else class="hosts-grid">
          <el-card
            v-for="host in filteredHosts"
            :key="host.host_id"
            class="host-card"
            shadow="hover"
          >
            <template #header>
              <div class="card-header">
                <div class="host-icon">
                  <el-icon :class="{ 'online': host.status === 'online', 'offline': host.status !== 'online' }">
                    <Monitor />
                  </el-icon>
                </div>
                <span class="host-name">{{ host.host_name }}</span>
              </div>
            </template>

            <div class="card-body">
              <div class="info-item">
                <span class="label">隧道 IP:</span>
                <span class="value tunnel-ip">{{ host.tunnel_ip || '-' }}</span>
              </div>
              
              <div class="info-item">
                <span class="label">SSH 用户:</span>
                <span class="value ssh-users">{{ formatSshUsers(host.ssh_users) }}</span>
              </div>

              <div class="info-item">
                <span class="label">状态:</span>
                <span class="value">
                  <el-tag v-if="host.status === 'online'" type="success" size="small">在线</el-tag>
                  <el-tag v-else type="info" size="small">离线</el-tag>
                </span>
              </div>

              <div class="info-item" v-if="host.last_seen">
                <span class="label">最后在线:</span>
                <span class="value time-text">{{ formatTime(host.last_seen) }}</span>
              </div>
            </div>
          </el-card>
        </div>
      </div>
    </div>
  </Layout>
</template>

<script setup lang="ts">
import { onMounted, ref, computed, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Search, Monitor } from '@element-plus/icons-vue'
import Layout from '../components/Layout.vue'
import { GetHosts } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/app'

interface Host {
  host_id: string
  host_name: string
  tunnel_ip: string
  ssh_users: string[]
  status: string
  last_seen: string
}

const loading = ref(false)
const hosts = ref<Host[]>([])
const searchQuery = ref('')
const searchExpanded = ref(false)
const searchInputRef = ref()

// 切换搜索框展开/收起
const toggleSearch = async () => {
  searchExpanded.value = !searchExpanded.value
  
  if (searchExpanded.value) {
    await nextTick()
    searchInputRef.value?.focus()
  } else {
    searchQuery.value = ''
  }
}

// 搜索框失去焦点时，如果没有搜索内容则自动收起
const handleSearchBlur = () => {
  if (!searchQuery.value) {
    setTimeout(() => {
      searchExpanded.value = false
    }, 200)
  }
}

// 过滤后的主机列表
const filteredHosts = computed(() => {
  if (!searchQuery.value) {
    return hosts.value
  }

  const query = searchQuery.value.toLowerCase()
  return hosts.value.filter(host => 
    host.host_name.toLowerCase().includes(query) ||
    host.tunnel_ip.toLowerCase().includes(query)
  )
})

const loadHosts = async () => {
  loading.value = true
  try {
    const result = await GetHosts()
    hosts.value = (result || []).filter((h): h is Host => h !== null)
  } catch (error: any) {
    ElMessage.error(error.message || '获取主机列表失败')
  } finally {
    loading.value = false
  }
}

const handleRefresh = async () => {
  await loadHosts()
  ElMessage.success('刷新成功')
}

const formatSshUsers = (users: string[]) => {
  if (!users || users.length === 0) return '-'
  return users.join(', ')
}

const formatTime = (timeStr: string) => {
  if (!timeStr) return '-'
  const date = new Date(timeStr)
  const now = new Date()
  const diff = now.getTime() - date.getTime()
  const seconds = Math.floor(diff / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)

  if (days > 0) return `${days}天前`
  if (hours > 0) return `${hours}小时前`
  if (minutes > 0) return `${minutes}分钟前`
  return '刚刚'
}

onMounted(() => {
  loadHosts()
})
</script>

<style scoped>
.hosts-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: #f5f5f5;
}

.page-header {
  background: white;
  padding: 20px 30px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-left h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 500;
  color: #333;
}

.header-right {
  display: flex;
  gap: 8px;
  align-items: center;
}

.search-input {
  width: 200px;
}

/* 搜索框展开/收起动画 */
.search-expand-enter-active,
.search-expand-leave-active {
  transition: all 0.3s ease;
}

.search-expand-enter-from {
  width: 0;
  opacity: 0;
  transform: translateX(20px);
}

.search-expand-enter-to {
  width: 200px;
  opacity: 1;
  transform: translateX(0);
}

.search-expand-leave-from {
  width: 200px;
  opacity: 1;
  transform: translateX(0);
}

.search-expand-leave-to {
  width: 0;
  opacity: 0;
  transform: translateX(20px);
}

.hosts-content {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
}

.hosts-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
  gap: 20px;
}

.host-card {
  transition: all 0.3s;
}

.host-card:hover {
  transform: translateY(-4px);
}

.card-header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.host-icon {
  font-size: 32px;
}

.host-icon .online {
  color: #67c23a;
}

.host-icon .offline {
  color: #909399;
}

.host-name {
  font-weight: bold;
  font-size: 16px;
  color: #333;
}

.card-body {
  padding: 10px 0;
}

.info-item {
  display: flex;
  justify-content: space-between;
  margin-bottom: 10px;
  font-size: 14px;
}

.info-item:last-child {
  margin-bottom: 0;
}

.info-item .label {
  color: #666;
  font-weight: 500;
}

.info-item .value {
  color: #333;
}

.info-item .value.tunnel-ip {
  color: #409eff;
  font-family: monospace;
  font-weight: 500;
}

.info-item .value.ssh-users {
  color: #e6a23c;
  font-family: monospace;
}

.time-text {
  font-size: 13px;
  color: #666;
}
</style>
