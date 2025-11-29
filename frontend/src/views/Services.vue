<template>
  <Layout>
    <div class="services-page">
      <!-- 页面头部 -->
      <div class="page-header">
        <div class="header-left">
          <h2>我的服务</h2>
          <el-tag v-if="!servicesStore.loading">
            共 {{ filteredServices.length }} 个服务
          </el-tag>
        </div>
        <div class="header-right">
          <el-input
            v-model="searchQuery"
            placeholder="搜索服务"
            :prefix-icon="Search"
            clearable
            style="width: 200px;"
          />
          <el-checkbox v-model="showOnlineOnly" label="在线" />
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
import { onMounted, ref, computed } from 'vue'
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
const showOnlineOnly = ref(false)

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

  // 按在线状态过滤（显示服务状态为online的）
  if (showOnlineOnly.value) {
    services = services.filter(service => service.status === 'online')
  }

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

.header-right {
  display: flex;
  gap: 10px;
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
