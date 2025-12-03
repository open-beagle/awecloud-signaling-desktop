<template>
  <Layout>
    <div class="services-page">
      <!-- 页面头部 -->
      <div class="page-header">
        <div class="header-left">
          <h2>我的服务</h2>
          <div class="filter-tags">
            <el-tag 
              :class="{ 'filter-tag': true, 'active': filterStatus.connected }"
              @click="toggleFilter('connected')"
              effect="plain"
            >
              共 {{ connectedCount }} 个已连接
            </el-tag>
            <el-tag 
              :class="{ 'filter-tag': true, 'active': filterStatus.online }"
              @click="toggleFilter('online')"
              type="success"
              effect="plain"
            >
              共 {{ onlineCount }} 个在线
            </el-tag>
            <el-tag 
              :class="{ 'filter-tag': true, 'active': filterStatus.offline }"
              @click="toggleFilter('offline')"
              type="info"
              effect="plain"
            >
              共 {{ offlineCount }} 个离线
            </el-tag>
          </div>
        </div>
        <div class="header-right">
          <!-- 搜索框（可展开/收起） -->
          <div class="search-wrapper">
            <transition name="search-expand">
              <el-input
                v-if="searchExpanded"
                ref="searchInputRef"
                v-model="searchQuery"
                placeholder="搜索服务"
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
          </div>
          
          <el-tooltip content="刷新" placement="bottom">
            <el-button 
              :icon="Refresh" 
              @click="handleRefresh" 
              :loading="servicesStore.loading"
              circle
            />
          </el-tooltip>
        </div>
      </div>

      <!-- 服务列表 -->
      <div class="services-content">
        <el-empty 
          v-if="!servicesStore.loading && servicesStore.services.length === 0" 
          description="暂无可用服务" 
        />
        
        <el-empty 
          v-else-if="!servicesStore.loading && filteredServices.length === 0" 
          description="没有找到匹配的服务" 
        />
        
        <div v-else class="services-grid">
          <ServiceCard
            v-for="service in filteredServices"
            :key="service.instance_id"
            :service="service"
            :connection="servicesStore.getConnectionStatus(service.instance_id)"
            @connect="handleConnect"
            @disconnect="handleDisconnect"
          />
        </div>
      </div>
    </div>
  </Layout>
</template>

<script setup lang="ts">
import { onMounted, ref, computed, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Search } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { useServicesStore } from '../stores/services'
import Layout from '../components/Layout.vue'
import ServiceCard from '../components/ServiceCard.vue'
import { GetServices, ConnectService, DisconnectService } from '../../wailsjs/go/main/App'

const authStore = useAuthStore()
const servicesStore = useServicesStore()

// 搜索和过滤
const searchQuery = ref('')
const searchExpanded = ref(false)
const searchInputRef = ref()
const filterStatus = ref({
  connected: true,  // 默认选中
  online: true,     // 默认选中
  offline: true     // 默认选中
})

// 切换搜索框展开/收起
const toggleSearch = async () => {
  searchExpanded.value = !searchExpanded.value
  
  if (searchExpanded.value) {
    // 展开时自动聚焦
    await nextTick()
    searchInputRef.value?.focus()
  } else {
    // 收起时清空搜索内容
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

// 统计数量
const connectedCount = computed(() => {
  return servicesStore.services.filter(service => {
    const connection = servicesStore.getConnectionStatus(service.instance_id)
    return connection?.status === 'connected'
  }).length
})

const onlineCount = computed(() => {
  return servicesStore.services.filter(service => service.status === 'online').length
})

const offlineCount = computed(() => {
  return servicesStore.services.filter(service => service.status !== 'online').length
})

// 切换筛选状态
const toggleFilter = (type: 'connected' | 'online' | 'offline') => {
  filterStatus.value[type] = !filterStatus.value[type]
}

// 过滤后的服务列表
const filteredServices = computed(() => {
  let services = servicesStore.services

  // 按搜索关键词过滤
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    services = services.filter(service => 
      service.instance_name.toLowerCase().includes(query) ||
      service.agent_name?.toLowerCase().includes(query) ||
      service.description?.toLowerCase().includes(query)
    )
  }

  // 按状态过滤
  services = services.filter(service => {
    const connection = servicesStore.getConnectionStatus(service.instance_id)
    const isConnected = connection?.status === 'connected'
    const isOnline = service.status === 'online'
    const isOffline = service.status !== 'online'

    // 如果已连接筛选开启，且服务已连接，则显示
    if (filterStatus.value.connected && isConnected) return true
    
    // 如果在线筛选开启，且服务在线（但未连接），则显示
    if (filterStatus.value.online && isOnline && !isConnected) return true
    
    // 如果离线筛选开启，且服务离线（且未连接），则显示
    if (filterStatus.value.offline && isOffline && !isConnected) return true

    return false
  })

  return services
})

onMounted(async () => {
  await loadServices()
})

const loadServices = async () => {
  servicesStore.setLoading(true)
  try {
    const services = await GetServices()
    servicesStore.setServices(services || [])
  } catch (error: any) {
    ElMessage.error(error.message || '获取服务列表失败')
  } finally {
    servicesStore.setLoading(false)
  }
}

const handleRefresh = async () => {
  await loadServices()
  ElMessage.success('刷新成功')
}

const handleConnect = async (instanceId: number, localPort: number) => {
  // 更新状态为连接中
  servicesStore.updateConnectionStatus(instanceId, {
    instance_id: instanceId,
    status: 'connecting',
    local_port: localPort
  })

  try {
    await ConnectService(instanceId, localPort)
    
    // 更新状态为已连接
    servicesStore.updateConnectionStatus(instanceId, {
      instance_id: instanceId,
      status: 'connected',
      local_port: localPort
    })
    
    ElMessage.success(`连接成功，本地端口: ${localPort}`)
  } catch (error: any) {
    // 更新状态为错误
    servicesStore.updateConnectionStatus(instanceId, {
      instance_id: instanceId,
      status: 'error',
      local_port: localPort,
      error: error.message
    })
    
    ElMessage.error(error.message || '连接失败')
  }
}

const handleDisconnect = async (instanceId: number) => {
  try {
    await DisconnectService(instanceId)
    
    // 更新状态为已断开
    servicesStore.updateConnectionStatus(instanceId, {
      instance_id: instanceId,
      status: 'disconnected',
      local_port: 0
    })
    
    ElMessage.success('已断开连接')
  } catch (error: any) {
    ElMessage.error(error.message || '断开连接失败')
  }
}


</script>

<style scoped>
.services-page {
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

.filter-tags {
  display: flex;
  gap: 8px;
}

.filter-tag {
  cursor: pointer;
  user-select: none;
  transition: all 0.3s ease;
  opacity: 0.5;
}

.filter-tag:hover {
  opacity: 0.8;
  transform: translateY(-1px);
}

.filter-tag.active {
  opacity: 1;
  background: linear-gradient(135deg, var(--el-tag-bg-color) 0%, var(--el-color-primary-light-7) 100%);
  border-color: var(--el-color-primary-light-5);
  font-weight: 500;
}

.header-right {
  display: flex;
  gap: 10px;
  align-items: center;
}

.search-wrapper {
  display: flex;
  align-items: center;
  gap: 10px;
  position: relative;
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

.services-content {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
}

.services-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
  gap: 20px;
}
</style>
