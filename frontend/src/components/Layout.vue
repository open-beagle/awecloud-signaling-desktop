<template>
  <div class="app-layout">
	<section v-if="updateStore.required" class="forced-update" role="alertdialog" aria-modal="true" aria-labelledby="forced-update-title">
	  <div class="forced-update-panel">
		<div class="forced-symbol">!</div>
		<div>
		  <h1 id="forced-update-title">需要升级后继续使用</h1>
		  <p>当前版本已低于平台最低支持版本。升级完成前不能建立新的资源连接。</p>
		  <div class="forced-version-flow">
			<span>当前版本 <strong>{{ updateStore.currentVersion }}</strong></span>
			<span>目标版本 <strong>{{ updateStore.targetVersion }}</strong></span>
		  </div>
		  <div class="forced-actions">
			<el-button @click="navigateTo('/logs')">连接诊断</el-button>
			<el-button type="primary" @click="openUpdateDialog">立即升级</el-button>
		  </div>
		</div>
	  </div>
	</section>
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

          <button class="update-entry" type="button" :class="{ active: updateStore.updateAvailable }" @click="checkDesktopUpdate">
            <span class="update-copy">
              <strong>客户端 {{ displayVersion }}</strong>
              <small>Commit {{ shortCommit(updateStore.currentCommitID) }}</small>
              <small>{{ formatCommitTime(updateStore.currentCommitTime) }}</small>
            </span>
            <el-icon v-if="updateStore.isChecking" class="is-loading update-loading"><Loading /></el-icon>
            <span v-else-if="updateStore.updateAvailable" class="update-badge" @click.stop="openUpdateDialog">更新</span>
            <span v-else class="update-state">{{ updateStore.checkError ? '重试' : '最新' }}</span>
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
        <UpdateModal
          ref="updateModalRef"
          :current-version="updateStore.currentVersion"
          :target-version="updateStore.targetVersion"
          :release-notes="updateStore.releaseNotes"
          :artifact-size="updateStore.artifactSize"
          :required="updateStore.required"
          :current-commit-id="updateStore.currentCommitID"
          :current-commit-time="updateStore.currentCommitTime"
          :target-commit-id="updateStore.manifest?.release.commit_id"
          :target-commit-time="updateStore.manifest?.release.published_at"
          :handing-off="updateStore.isHandingOff"
          @request-update="handleRequestUpdate"
          @browser-download="handleBrowserDownload"
        />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, markRaw, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  ArrowUp,
  Box,
  Compass,
  Connection,
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
import { useUpdateStore } from '../stores/update'
import type { DomainItem } from '../stores/domains'
import UpdateModal from './update/UpdateModal.vue'
import {
  ClearCredentials,
  ApplyDesktopUpdate,
  CheckDesktopUpdate,
  GetDomainList,
  GetGRPCStatus,
  GetTunnelStatus,
  GetVersion,
  Logout,
  OpenBrowser
} from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/internal/app/app'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const servicesStore = useServicesStore()
const domainsStore = useDomainsStore()
const updateStore = useUpdateStore()
const updateModalRef = ref<InstanceType<typeof UpdateModal> | null>(null)
const displayVersion = computed(() => {
  const version = updateStore.currentVersion || '-'
  return version === '-' || version.startsWith('v') ? version : `v${version}`
})
const shortCommit = (value: string) => value ? value.slice(0, 8) : '-'
const formatCommitTime = (value: string) => {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? (value || '-') : date.toLocaleString([], { hour12: false })
}

const openUpdateDialog = () => updateModalRef.value?.show()

const checkDesktopUpdate = async () => {
  if (updateStore.isChecking) return
  try {
    updateStore.isChecking = true
    updateStore.checkError = ''
    const state = await CheckDesktopUpdate()
    if (state) updateStore.applyCheck(state as any)
  } catch (error: any) {
    updateStore.checkError = error?.message || '检查更新失败'
    console.error('Failed to check Desktop update:', error)
  } finally {
    updateStore.isChecking = false
  }
}

const handleRequestUpdate = async () => {
  try {
    const manifest = updateStore.manifest
    if (!manifest?.artifacts?.app) throw new Error('没有可用的 Desktop 更新制品')
    updateStore.isHandingOff = true
    const accepted = await ApplyDesktopUpdate({
      schema_version: 1,
      request_id: crypto.randomUUID(),
      force: manifest.update.required,
      target_version: manifest.release.version,
      manifest: manifest,
      artifact: manifest.artifacts.app
    })
    if (!accepted?.accepted) throw new Error('Launcher 未接受更新任务')
    ElMessage.success('Launcher 已接管，客户端正在退出')
  } catch (err: any) {
    updateStore.isHandingOff = false
    ElMessage.error(err.message || '更新请求失败')
  }
}

const handleBrowserDownload = async () => {
  const url = updateStore.availableManifest?.artifacts?.app?.download_url
  if (!url) return
  try { await OpenBrowser(url) } catch (err: any) { ElMessage.error(err?.message || '无法打开浏览器') }
}

const accessNavigation = [
  { path: '/hosts', label: 'SSH', icon: markRaw(Monitor) },
  { path: '/k8s', label: 'Kubernetes', icon: markRaw(Compass) },
  { path: '/services', label: 'Kubernetes SVC', icon: markRaw(Connection) },
  { path: '/containers', label: 'Kubernetes Pods', icon: markRaw(Box) }
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

onMounted(async () => {
  loadConnectionStatus()
  loadDomains()
  tunnelTimer = window.setInterval(loadTunnelStatus, 5000)
  grpcTimer = window.setInterval(loadGRPCStatus, 5000)
  domainsTimer = window.setInterval(loadDomains, 30000)
  try {
    const version = await GetVersion()
    if (version) updateStore.setLocalVersion(version.version, version.gitCommit, version.commitTime)
  } catch (error) {
    console.error('Failed to load local Desktop version:', error)
  }
  await checkDesktopUpdate()
})

onUnmounted(() => {
  if (tunnelTimer) clearInterval(tunnelTimer)
  if (grpcTimer) clearInterval(grpcTimer)
  if (domainsTimer) clearInterval(domainsTimer)
})

watch(() => updateStore.modalRequest, async () => {
  if (!updateStore.updateAvailable) await checkDesktopUpdate()
  if (updateStore.updateAvailable) openUpdateDialog()
})

const isActive = (path: string) => {
  return route.path === path || route.path.startsWith(`${path}/`)
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

.forced-update {
  position: fixed;
  z-index: 20;
  inset: 44px 0 0;
  display: grid;
  place-items: center;
  padding: 32px;
  background: rgba(244, 246, 249, 0.97);
}

.forced-update-panel {
  width: min(620px, 100%);
  display: grid;
  grid-template-columns: 52px 1fr;
  gap: 22px;
  padding: 30px;
  background: #fff;
  border: 1px solid #dbe2ec;
  border-radius: 10px;
  box-shadow: 0 18px 52px rgba(40, 58, 88, 0.14);
}

.forced-symbol { width: 48px; height: 48px; display: grid; place-items: center; color: #fff; background: #365ca8; border-radius: 50%; font-size: 24px; font-weight: 700; }
.forced-update h1 { margin: 0; color: #172033; font-size: 22px; }
.forced-update p { margin: 9px 0 20px; color: #66758a; font-size: 13px; line-height: 1.7; }
.forced-version-flow { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.forced-version-flow span { display: flex; flex-direction: column; gap: 5px; padding: 13px; color: #7b8799; background: #f5f7fa; border: 1px solid #e2e7ef; border-radius: 7px; font-size: 11px; }
.forced-version-flow strong { color: #284f9c; font-size: 16px; }
.forced-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 22px; }

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
.update-entry,
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
.update-entry:hover,
.account-menu:hover {
  background: #f5f7fa;
}

.update-entry {
  min-height: 50px;
  justify-content: space-between;
  gap: 8px;
  margin-top: 3px;
  padding: 7px 10px;
  color: #606f85;
  text-align: left;
}

.update-entry.active {
  color: #284f9c;
  background: #eef3fc;
}

.update-copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.update-copy strong,
.update-copy small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.update-copy strong { color: #4f5c70; font-size: 12px; }
.update-copy small { max-width: 132px; color: #8a95a7; font-size: 11px; }
.update-entry.active .update-copy strong { color: #284f9c; }

.update-badge {
  flex: 0 0 auto;
  padding: 2px 7px;
  color: #fff;
  background: #365ca8;
  border-radius: 10px;
  font-size: 10px;
  font-weight: 600;
}

.update-loading { flex: 0 0 auto; color: #365ca8; font-size: 16px; }
.update-state { flex: 0 0 auto; color: #8a95a7; font-size: 10px; }

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
