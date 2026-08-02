<template>
  <div class="app-layout">
    <header class="titlebar">
      <div class="brand">
        <img src="../assets/logo.png" alt="Beagle Signal" class="logo" />
        <span>Beagle Signal</span>
      </div>
      <span class="window-title">安全访问客户端</span>
    </header>

    <div class="app-shell">
      <aside class="sidebar">
        <div class="identity">
          <span class="identity-label">当前身份</span>
          <strong :title="authStore.clientId">{{ authStore.clientId || '已登录用户' }}</strong>
          <small>实名用户</small>
        </div>

        <nav class="nav" aria-label="Desktop 主导航">
          <span class="nav-label">访问</span>
          <button
            v-for="item in accessNavigation"
            :key="item.path"
            class="nav-item"
            :class="{ active: isActive(item.path) }"
            type="button"
            @click="navigateTo(item.path)"
          >
            <el-icon><component :is="item.icon" /></el-icon>
            <span>{{ item.label }}</span>
          </button>

          <span class="nav-label account-label">账号与客户端</span>
          <button
            v-for="item in accountNavigation"
            :key="item.path"
            class="nav-item"
            :class="{ active: isActive(item.path) }"
            type="button"
            @click="navigateTo(item.path)"
          >
            <el-icon><component :is="item.icon" /></el-icon>
            <span>{{ item.label }}</span>
          </button>
        </nav>

        <div class="sidebar-footer">
          <button class="connection-summary" type="button" @click="navigateTo('/logs')">
            <span class="connection-copy">
              <strong>连接状态</strong>
              <small>{{ connectionDescription }}</small>
            </span>
            <el-icon v-if="connectionLoading" class="is-loading status-loading"><Loading /></el-icon>
            <span v-else class="status-dot" :class="connectionTone" aria-hidden="true"></span>
          </button>

          <el-dropdown trigger="click" placement="top-start" @command="handleUserCommand">
            <button class="account-menu" type="button">
              <el-icon><User /></el-icon>
              <span>账号操作</span>
              <el-icon class="account-chevron"><ArrowUp /></el-icon>
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="logout">
                  <el-icon><SwitchButton /></el-icon>
                  <span>注销</span>
                </el-dropdown-item>
                <el-dropdown-item command="switchUser">
                  <el-icon><User /></el-icon>
                  <span>切换用户</span>
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </aside>

      <main class="main-content">
        <router-view />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, markRaw, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  ArrowUp,
  Collection,
  Compass,
  Document,
  Iphone,
  Loading,
  Monitor,
  SwitchButton,
  User
} from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { useServicesStore } from '../stores/services'
import { useDomainsStore } from '../stores/domains'
import type { DomainItem } from '../stores/domains'
import {
  ClearCredentials,
  GetDomainList,
  GetGRPCStatus,
  GetTunnelStatus,
  Logout
} from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/app'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const servicesStore = useServicesStore()
const domainsStore = useDomainsStore()

const accessNavigation = [
  { path: '/resources', label: '资源', icon: markRaw(Collection) },
  { path: '/hosts', label: 'SSH', icon: markRaw(Monitor) },
  { path: '/k8s', label: 'Kubernetes', icon: markRaw(Compass) }
]

const accountNavigation = [
  { path: '/devices', label: '我的设备', icon: markRaw(Iphone) },
  { path: '/logs', label: '连接诊断', icon: markRaw(Document) }
]

const tunnelStatus = ref({ connected: false, ip: '', error: '' })
const grpcStatus = ref({ connected: false, server_address: '', error: '' })
const connectionLoading = ref(true)
let tunnelTimer: number | null = null
let grpcTimer: number | null = null
let domainsTimer: number | null = null

const connectionDescription = computed(() => {
  if (connectionLoading.value) return '正在检测'
  if (grpcStatus.value.connected && tunnelStatus.value.connected) {
    return tunnelStatus.value.ip || '服务与网络已连接'
  }
  if (grpcStatus.value.connected || tunnelStatus.value.connected) return '部分连接可用'
  return '连接中断'
})

const connectionTone = computed(() => {
  if (grpcStatus.value.connected && tunnelStatus.value.connected) return 'connected'
  if (grpcStatus.value.connected || tunnelStatus.value.connected) return 'warning'
  return 'disconnected'
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

const loadConnectionStatus = async () => {
  await Promise.all([loadTunnelStatus(), loadGRPCStatus()])
  connectionLoading.value = false
}

const loadDomains = async () => {
  try {
    const domains = await GetDomainList()
    domainsStore.setDomains(((domains || []).filter(Boolean)) as DomainItem[])
  } catch (error) {
    console.error('Failed to get domain list:', error)
  }
}

onMounted(() => {
  loadConnectionStatus()
  loadDomains()
  tunnelTimer = window.setInterval(loadTunnelStatus, 5000)
  grpcTimer = window.setInterval(loadGRPCStatus, 5000)
  domainsTimer = window.setInterval(loadDomains, 30000)
})

onUnmounted(() => {
  if (tunnelTimer) clearInterval(tunnelTimer)
  if (grpcTimer) clearInterval(grpcTimer)
  if (domainsTimer) clearInterval(domainsTimer)
})

const isActive = (path: string) => {
  return route.path === path || (path !== '/resources' && route.path.startsWith(`${path}/`))
}

const navigateTo = (path: string) => {
  if (route.path !== path) router.push(path)
}

const handleUserCommand = async (command: string) => {
  if (command === 'switchUser') {
    await Logout()
    await ClearCredentials()
    authStore.logout()
    servicesStore.clearConnections()
    router.push('/login')
    ElMessage.success('已清除登录状态，请使用其他账号登录')
  } else if (command === 'logout') {
    await Logout()
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
  color: #303133;
  background: #f4f6f8;
}

.titlebar {
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex: 0 0 44px;
  padding: 0 16px;
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  user-select: none;
}

.brand {
  display: flex;
  align-items: center;
  gap: 9px;
  font-size: 14px;
  font-weight: 650;
}

.logo {
  width: 26px;
  height: 26px;
  object-fit: contain;
}

.window-title {
  color: #909399;
  font-size: 12px;
}

.app-shell {
  min-height: 0;
  display: flex;
  flex: 1;
}

.sidebar {
  width: 216px;
  display: flex;
  flex: 0 0 216px;
  flex-direction: column;
  padding: 14px 10px 10px;
  background: #fff;
  border-right: 1px solid #e4e7ed;
}

.identity {
  min-width: 0;
  padding: 10px 10px 12px;
  border-bottom: 1px solid #ebeef5;
}

.identity-label,
.nav-label {
  display: block;
  color: #909399;
  font-size: 11px;
}

.identity strong {
  display: block;
  overflow: hidden;
  margin-top: 4px;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.identity small {
  display: block;
  margin-top: 3px;
  color: #909399;
  font-size: 11px;
}

.nav {
  padding-top: 10px;
}

.nav-label {
  margin: 8px 11px 5px;
  font-weight: 600;
}

.account-label {
  margin-top: 18px;
}

.nav-item {
  width: 100%;
  height: 40px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 11px;
  color: #606266;
  background: transparent;
  border: 0;
  border-radius: 8px;
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
  transition: color 0.15s, background-color 0.15s, transform 0.15s;
}

.nav-item:hover {
  color: #409eff;
  background: #f1f5f9;
}

.nav-item:active,
.account-menu:active,
.connection-summary:active {
  transform: translateY(1px);
}

.nav-item:focus-visible,
.account-menu:focus-visible,
.connection-summary:focus-visible {
  outline: 2px solid #409eff;
  outline-offset: 2px;
}

.nav-item.active {
  color: #409eff;
  background: #ecf5ff;
  font-weight: 600;
}

.nav-item .el-icon {
  font-size: 18px;
}

.sidebar-footer {
  margin-top: auto;
  padding-top: 10px;
  border-top: 1px solid #ebeef5;
}

.connection-summary,
.account-menu {
  width: 100%;
  display: flex;
  align-items: center;
  background: transparent;
  border: 0;
  border-radius: 8px;
  cursor: pointer;
  transition: background-color 0.15s, transform 0.15s;
}

.connection-summary {
  justify-content: space-between;
  padding: 8px 10px;
  text-align: left;
}

.connection-summary:hover,
.account-menu:hover {
  background: #f5f7fa;
}

.connection-copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.connection-copy strong {
  color: #606266;
  font-size: 12px;
}

.connection-copy small {
  max-width: 145px;
  overflow: hidden;
  color: #909399;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status-dot {
  width: 8px;
  height: 8px;
  flex: 0 0 8px;
  border-radius: 50%;
}

.status-dot.connected {
  background: #67c23a;
}

.status-dot.warning {
  background: #e6a23c;
}

.status-dot.disconnected {
  background: #f56c6c;
}

.status-loading {
  color: #909399;
}

.account-menu {
  height: 38px;
  gap: 8px;
  margin-top: 4px;
  padding: 0 10px;
  color: #606266;
  font-size: 12px;
}

.account-chevron {
  margin-left: auto;
}

.main-content {
  min-width: 0;
  min-height: 0;
  display: flex;
  flex: 1;
  flex-direction: column;
  overflow: auto;
}
</style>
