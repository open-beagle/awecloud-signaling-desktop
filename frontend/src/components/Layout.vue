<template>
  <div class="app-layout">
    <!-- 顶部导航栏 -->
    <div class="navbar">
      <div class="navbar-left">
        <img src="../assets/logo.png" alt="Logo" class="logo" />
        <span class="app-name">信令桌面</span>
        
        <!-- gRPC 状态 -->
        <el-tooltip :content="grpcTooltip" placement="bottom">
          <div class="status-indicator grpc-status">
            <el-icon v-if="grpcStatus.connected" class="status-icon connected"><CircleCheck /></el-icon>
            <el-icon v-else-if="grpcReconnecting" class="status-icon reconnecting is-loading"><Loading /></el-icon>
            <el-icon v-else class="status-icon disconnected"><CircleClose /></el-icon>
            <span class="status-text">gRPC</span>
          </div>
        </el-tooltip>
        
        <!-- 隧道状态 -->
        <el-tooltip :content="tunnelTooltip" placement="bottom">
          <div class="tunnel-status" @click="handleTunnelClick">
            <el-icon v-if="tunnelLoading" class="is-loading tunnel-icon"><Loading /></el-icon>
            <el-icon v-else-if="tunnelStatus.connected" class="tunnel-icon connected"><CircleCheck /></el-icon>
            <el-icon v-else class="tunnel-icon disconnected"><CircleClose /></el-icon>
            <span v-if="tunnelStatus.connected" class="tunnel-ip">{{ tunnelStatus.ip }}</span>
            <span v-else class="tunnel-text">Tunnel</span>
          </div>
        </el-tooltip>
      </div>
      
      <div class="navbar-right">
        <!-- 我的服务 -->
        <div 
          class="nav-item"
          :class="{ active: currentRoute === '/services' }"
          @click="navigateTo('/services')"
        >
          <el-icon><Grid /></el-icon>
          <span>我的服务</span>
        </div>
        
        <!-- 我的主机 -->
        <div 
          class="nav-item"
          :class="{ active: currentRoute.startsWith('/hosts') }"
          @click="navigateTo('/hosts')"
        >
          <el-icon><Monitor /></el-icon>
          <span>我的主机</span>
        </div>
        
        <!-- 用户菜单 -->
        <el-dropdown trigger="hover" @command="handleUserCommand">
          <div class="user-menu">
            <el-icon class="user-icon"><User /></el-icon>
          </div>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item disabled>
                <div class="user-info">
                  <el-icon><User /></el-icon>
                  <span>{{ authStore.clientId }}</span>
                </div>
              </el-dropdown-item>
              <el-dropdown-item divided command="devices">
                <el-icon><Iphone /></el-icon>
                <span>我的设备</span>
              </el-dropdown-item>
              <el-dropdown-item command="logs">
                <el-icon><Document /></el-icon>
                <span>查看日志</span>
              </el-dropdown-item>
              <el-dropdown-item divided command="logout">
                <el-icon><SwitchButton /></el-icon>
                <span>注销</span>
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </div>
    
    <!-- 主内容区域 -->
    <div class="main-content">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Grid, Document, Monitor, User, SwitchButton, CircleCheck, CircleClose, Loading, Iphone } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { useServicesStore } from '../stores/services'
import { GetTunnelStatus, ReconnectTunnel, GetGRPCStatus, Logout } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/app'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const servicesStore = useServicesStore()

// 隧道状态
const tunnelStatus = ref({ connected: false, ip: '', error: '' })
const tunnelLoading = ref(false)
let tunnelTimer: number | null = null

// gRPC 状态
const grpcStatus = ref({ connected: false, server_address: '', error: '' })
let grpcTimer: number | null = null
// gRPC 重连中状态：之前连接过但现在断开
const grpcWasConnected = ref(false)
const grpcReconnecting = computed(() => {
  return grpcWasConnected.value && !grpcStatus.value.connected
})

const tunnelTooltip = computed(() => {
  if (tunnelLoading.value) return '正在连接隧道...'
  if (tunnelStatus.value.connected) {
    return `隧道已连接\n点击可重连`
  }
  return `${tunnelStatus.value.error || '隧道未连接'}\n点击连接`
})

const grpcTooltip = computed(() => {
  if (grpcStatus.value.connected) {
    return `gRPC 已连接\n服务器: ${grpcStatus.value.server_address}`
  }
  if (grpcReconnecting.value) {
    return `gRPC 重连中...\n${grpcStatus.value.error || ''}`
  }
  return `gRPC 未连接\n${grpcStatus.value.error || ''}`
})

const loadTunnelStatus = async () => {
  try {
    const status = await GetTunnelStatus()
    if (status) {
      tunnelStatus.value = {
        connected: status.connected,
        ip: status.ip || '',
        error: status.error || ''
      }
    }
  } catch (error) {
    console.error('Failed to get tunnel status:', error)
  }
}

const loadGRPCStatus = async () => {
  try {
    const status = await GetGRPCStatus()
    if (status) {
      if (status.connected) {
        grpcWasConnected.value = true
      }
      grpcStatus.value = {
        connected: status.connected,
        server_address: status.server_address || '',
        error: status.error || ''
      }
    }
  } catch (error) {
    console.error('Failed to get gRPC status:', error)
  }
}

const handleTunnelClick = async () => {
  if (tunnelLoading.value) return
  tunnelLoading.value = true
  try {
    await ReconnectTunnel()
    await loadTunnelStatus()
    if (tunnelStatus.value.connected) {
      ElMessage.success(`隧道已连接: ${tunnelStatus.value.ip}`)
    } else {
      ElMessage.error(tunnelStatus.value.error || '连接失败')
    }
  } catch (error: any) {
    ElMessage.error(error.message || '重连失败')
    await loadTunnelStatus()
  } finally {
    tunnelLoading.value = false
  }
}

onMounted(() => {
  loadTunnelStatus()
  loadGRPCStatus()
  tunnelTimer = window.setInterval(loadTunnelStatus, 5000)
  grpcTimer = window.setInterval(loadGRPCStatus, 5000)
})

onUnmounted(() => {
  if (tunnelTimer) {
    clearInterval(tunnelTimer)
  }
  if (grpcTimer) {
    clearInterval(grpcTimer)
  }
})

const currentRoute = computed(() => route.path)

const navigateTo = (path: string) => {
  if (currentRoute.value !== path) {
    router.push(path)
  }
}

const handleUserCommand = (command: string) => {
  if (command === 'devices') {
    navigateTo('/devices')
  } else if (command === 'logs') {
    navigateTo('/logs')
  } else if (command === 'logout') {
    Logout()
    authStore.logout()
    servicesStore.clearConnections()
    router.push('/login')
    ElMessage.success('已退出登录')
  }
}
</script>

<style scoped>
.app-layout {
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: #f5f5f5;
}

.navbar {
  background: white;
  height: 60px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 24px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  z-index: 100;
}

.navbar-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo {
  width: 32px;
  height: 32px;
  object-fit: contain;
}

.app-name {
  font-size: 16px;
  font-weight: 500;
  color: #333;
}

.status-indicator {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  border-radius: 4px;
  margin-left: 8px;
}

.grpc-status {
  cursor: default;
}

.status-icon {
  font-size: 14px;
}

.status-icon.connected {
  color: #67c23a;
}

.status-icon.disconnected {
  color: #f56c6c;
}

.status-icon.reconnecting {
  color: #e6a23c;
}

.status-text {
  font-size: 12px;
  color: #666;
  font-weight: 500;
}

.tunnel-status {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
  margin-left: 8px;
}

.tunnel-status:hover {
  background: #f5f5f5;
}

.tunnel-icon {
  font-size: 14px;
}

.tunnel-icon.connected {
  color: #67c23a;
}

.tunnel-icon.disconnected {
  color: #f56c6c;
}

.tunnel-ip {
  font-size: 12px;
  color: #67c23a;
  font-family: monospace;
}

.tunnel-text {
  font-size: 12px;
  color: #666;
  font-weight: 500;
}

.navbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
  color: #666;
  font-size: 14px;
  user-select: none;
}

.nav-item:hover {
  background: #f5f5f5;
  color: #409eff;
}

.nav-item.active {
  background: #e6f4ff;
  color: #409eff;
  font-weight: 500;
}

.nav-item .el-icon {
  font-size: 18px;
}

.user-menu {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 50%;
  cursor: pointer;
  transition: all 0.2s;
  color: #666;
}

.user-menu:hover {
  background: #f5f5f5;
  color: #409eff;
}

.user-icon {
  font-size: 20px;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
  color: #333;
  font-weight: 500;
}

.main-content {
  flex: 1;
  overflow: hidden;
}
</style>
