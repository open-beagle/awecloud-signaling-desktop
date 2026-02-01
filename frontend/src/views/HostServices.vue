<template>
  <Layout>
    <div class="host-services-page">
      <!-- 页面头部 -->
      <div class="page-header">
        <div class="header-left">
          <el-button :icon="ArrowLeft" @click="goBack" circle />
          <div class="host-info">
            <h2>{{ hostName }}</h2>
            <div class="host-meta">
              <span class="tunnel-ip" v-if="tunnelIP">{{ tunnelIP }}</span>
              <el-tag v-if="hostStatus === 'online'" type="success" size="small">在线</el-tag>
              <el-tag v-else type="info" size="small">离线</el-tag>
            </div>
          </div>
        </div>
        <div class="header-right">
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

      <!-- 服务列表 -->
      <div class="services-content">
        <el-empty 
          v-if="!loading && services.length === 0" 
          description="该主机暂无服务" 
        />
        
        <div v-else class="services-grid">
          <ServiceCard
            v-for="(service, index) in services"
            :key="service.instance_id"
            :service="service"
            :index="index + 1"
          />
        </div>
      </div>
    </div>
  </Layout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, Refresh } from '@element-plus/icons-vue'
import Layout from '../components/Layout.vue'
import ServiceCard from '../components/ServiceCard.vue'
import { GetHostServices } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/app'
import type { ServiceInfo } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/models'

const router = useRouter()
const route = useRoute()

const loading = ref(false)
const services = ref<ServiceInfo[]>([])
const hostName = ref('')
const tunnelIP = ref('')
const hostStatus = ref('offline')

const hostId = route.params.id as string

const loadHostServices = async () => {
  loading.value = true
  try {
    const result = await GetHostServices(hostId)
    services.value = (result || []).filter(s => s !== null) as ServiceInfo[]
    
    // 从第一个服务获取主机信息
    if (services.value.length > 0) {
      const firstService = services.value[0]
      hostName.value = firstService.agent_name || '未知主机'
      tunnelIP.value = firstService.agent_tailscale_ip || ''
      hostStatus.value = firstService.status || 'offline'
    }
  } catch (error: any) {
    ElMessage.error(error.message || '获取服务列表失败')
  } finally {
    loading.value = false
  }
}

const handleRefresh = async () => {
  await loadHostServices()
  ElMessage.success('刷新成功')
}

const goBack = () => {
  router.push('/hosts')
}

onMounted(() => {
  loadHostServices()
})
</script>

<style scoped>
.host-services-page {
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
  gap: 16px;
}

.host-info {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.host-info h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 500;
  color: #333;
}

.host-meta {
  display: flex;
  align-items: center;
  gap: 12px;
}

.tunnel-ip {
  font-size: 13px;
  color: #409eff;
  font-family: monospace;
  font-weight: 500;
}

.header-right {
  display: flex;
  gap: 8px;
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
