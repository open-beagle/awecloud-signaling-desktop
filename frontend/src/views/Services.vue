<template>
  <div class="services-container">
    <!-- 顶部栏 -->
    <div class="header">
      <div class="header-left">
        <img src="../assets/logo.png" alt="Logo" class="logo" />
        <h2>我的服务</h2>
        <el-tag type="success" v-if="authStore.isAuthenticated">
          {{ authStore.clientId }}
        </el-tag>
      </div>
      <div class="header-right">
        <el-button :icon="Refresh" @click="handleRefresh" :loading="servicesStore.loading">
          刷新
        </el-button>
        <el-button @click="handleLogout">退出登录</el-button>
      </div>
    </div>

    <!-- 服务列表 -->
    <div class="services-list">
      <el-empty v-if="!servicesStore.loading && servicesStore.services.length === 0" description="暂无可用服务" />
      
      <div v-else class="services-grid">
        <ServiceCard
          v-for="service in servicesStore.services"
          :key="service.instance_id"
          :service="service"
          :connection="servicesStore.getConnectionStatus(service.instance_id)"
          @connect="handleConnect"
          @disconnect="handleDisconnect"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { useServicesStore } from '../stores/services'
import ServiceCard from '../components/ServiceCard.vue'
import { GetServices, ConnectService, DisconnectService, Logout } from '../../wailsjs/go/main/App'

const router = useRouter()
const authStore = useAuthStore()
const servicesStore = useServicesStore()

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

const handleLogout = () => {
  Logout()
  authStore.logout()
  servicesStore.clearConnections()
  router.push('/login')
  ElMessage.success('已退出登录')
}
</script>

<style scoped>
.services-container {
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: #f5f5f5;
}

.header {
  background: white;
  padding: 20px 30px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 15px;
}

.header-left .logo {
  width: 40px;
  height: 40px;
  object-fit: contain;
}

.header-left h2 {
  margin: 0;
  font-size: 24px;
  color: #333;
}

.header-right {
  display: flex;
  gap: 10px;
}

.services-list {
  flex: 1;
  padding: 30px;
  overflow-y: auto;
}

.services-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
  gap: 20px;
}
</style>
