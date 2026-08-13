<template>
  <div class="logs-page">
    <div class="page-header">
      <div class="header-left">
        <div>
          <h2>连接诊断</h2>
          <p>查看服务同步、安全网络、本地代理和客户端日志。</p>
        </div>
        <el-tag v-if="logs.length > 0">
          共 {{ logs.length }} 条
        </el-tag>
      </div>
      <div class="header-right">
        <el-tooltip :content="autoRefresh ? '自动刷新中（点击关闭）' : '点击开启自动刷新'" placement="bottom">
          <el-button 
            :icon="Refresh" 
            :type="autoRefresh ? 'primary' : 'default'"
            aria-label="切换日志自动刷新"
            @click="toggleAutoRefresh"
            circle
          />
        </el-tooltip>
        <el-tooltip content="滚动到底部" placement="bottom">
          <el-button 
            :icon="Bottom" 
            aria-label="滚动到日志底部"
            @click="scrollToBottom"
            circle
          />
        </el-tooltip>
        <el-tooltip content="清空" placement="bottom">
          <el-button 
            :icon="Delete" 
            aria-label="清空日志"
            @click="handleClear"
            circle
          />
        </el-tooltip>
        <el-tooltip content="下载" placement="bottom">
          <el-button 
            :icon="Download" 
            aria-label="下载日志"
            @click="handleDownload"
            circle
          />
        </el-tooltip>
      </div>
    </div>

    <div class="diagnostic-summary" v-loading="statusLoading">
      <div class="diagnostic-item">
        <span class="diagnostic-label">服务同步</span>
        <strong>{{ grpcStatus.connected ? '正常' : '未连接' }}</strong>
        <small>{{ grpcStatus.server_address || grpcStatus.error || '等待服务状态' }}</small>
      </div>
      <div class="diagnostic-item">
        <span class="diagnostic-label">安全网络</span>
        <strong>{{ tunnelStatus.connected ? '已连接' : '未连接' }}</strong>
        <small>{{ tunnelStatus.ip || tunnelStatus.error || '等待网络状态' }}</small>
        <el-button
          v-if="!tunnelStatus.connected"
          class="reconnect-button"
          size="small"
          :loading="reconnecting"
          @click="handleReconnect"
        >
          重新连接
        </el-button>
      </div>
      <div class="diagnostic-item">
        <span class="diagnostic-label">本地代理</span>
        <strong>{{ proxyStatus.length }} 个活动监听</strong>
        <small>{{ proxyStatus.length ? '按已访问资源建立' : '访问资源时自动建立' }}</small>
      </div>
    </div>

    <div class="version-section-title">
      <h3>客户端版本</h3>
      <span>{{ updateStore.checkedAt ? '最近检查：' + formatCheckedAt(updateStore.checkedAt) : '等待检查' }}</span>
    </div>
    <div class="client-version-row">
      <div>
        <strong>客户端版本</strong>
        <small>{{ platformLabel }}</small>
      </div>
      <div>
        <strong>{{ displayVersion }}</strong>
        <small>Commit {{ shortCommit(updateStore.currentCommitID) }} · {{ formatCommitTime(updateStore.currentCommitTime) }}</small>
        <small>{{ updateStore.updateAvailable ? `新版本 ${updateStore.targetVersion} 已发布` : updateStore.checkError || '当前已是最新版本' }}</small>
      </div>
      <el-button :type="updateStore.updateAvailable ? 'primary' : 'default'" :loading="updateStore.isChecking" @click="updateStore.openUpdate()">
        {{ updateStore.updateAvailable ? '查看更新' : '检查更新' }}
      </el-button>
    </div>

    <div class="log-section-title">
      <h3>客户端日志</h3>
      <span>仅保留本次运行期间的事件</span>
    </div>

    <div class="logs-content" ref="logsContainer">
      <div v-if="logs.length === 0" class="empty">
        暂无日志
      </div>
      <div v-else class="log-lines">
        <div
          v-for="(log, index) in logs"
          :key="index"
          class="log-line"
          :class="getLogClass(log)"
        >
          {{ log }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Delete, Download, Bottom } from '@element-plus/icons-vue'
import { useUpdateStore } from '../stores/update'
import {
  GetGRPCStatus,
  GetLogs,
  GetProxyStatus,
  GetTunnelStatus,
  ReconnectTunnel
} from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/internal/app/app'

const logs = ref<string[]>([])
const updateStore = useUpdateStore()
const displayVersion = computed(() => {
  const value = updateStore.currentVersion || '-'
  return value === '-' || value.startsWith('v') ? value : `v${value}`
})
const shortCommit = (value: string) => value ? value.slice(0, 8) : '-'
const formatCommitTime = (value: string) => {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? (value || '-') : date.toLocaleString([], { hour12: false })
}
const platformLabel = `${navigator.platform || 'Desktop'} · stable`
const formatCheckedAt = (value: string) => {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '刚刚' : date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}
const logsContainer = ref<HTMLElement | null>(null)
const autoRefresh = ref(true)
const isUserAtBottom = ref(true)
let refreshInterval: number | null = null
let statusInterval: number | null = null
const statusLoading = ref(true)
const reconnecting = ref(false)
const grpcStatus = ref({ connected: false, server_address: '', error: '' })
const tunnelStatus = ref({ connected: false, ip: '', error: '' })
const proxyStatus = ref<any[]>([])

const loadStatus = async () => {
  try {
    const [grpc, tunnel, proxies] = await Promise.all([
      GetGRPCStatus(),
      GetTunnelStatus(),
      GetProxyStatus()
    ])
    grpcStatus.value = {
      connected: Boolean(grpc?.connected),
      server_address: grpc?.server_address || '',
      error: grpc?.error || ''
    }
    tunnelStatus.value = {
      connected: Boolean(tunnel?.connected),
      ip: tunnel?.ip || '',
      error: tunnel?.error || ''
    }
    proxyStatus.value = (proxies || []).filter(Boolean)
  } catch (error: any) {
    ElMessage.error('加载连接状态失败: ' + (error?.message || error))
  } finally {
    statusLoading.value = false
  }
}

const handleReconnect = async () => {
  if (reconnecting.value) return
  reconnecting.value = true
  try {
    await ReconnectTunnel()
    await loadStatus()
    if (tunnelStatus.value.connected) {
      ElMessage.success('安全网络已重新连接')
    } else {
      ElMessage.error(tunnelStatus.value.error || '安全网络连接失败')
    }
  } catch (error: any) {
    ElMessage.error(error?.message || '安全网络重连失败')
  } finally {
    reconnecting.value = false
  }
}

// 检查用户是否在底部（允许 50px 误差）
const checkIfAtBottom = () => {
  const container = logsContainer.value
  if (!container) return true
  const threshold = 50
  return container.scrollHeight - container.scrollTop - container.clientHeight < threshold
}

// 滚动到底部
const scrollToBottom = () => {
  const container = logsContainer.value
  if (container) {
    container.scrollTop = container.scrollHeight
    isUserAtBottom.value = true
  }
}

const loadLogs = async (forceScroll = false) => {
  try {
    const result = await GetLogs()
    logs.value = result || []
    
    // 只有用户在底部时才自动滚动，或者强制滚动
    if (forceScroll || isUserAtBottom.value) {
      setTimeout(() => {
        scrollToBottom()
      }, 50)
    }
  } catch (error: any) {
    ElMessage.error('加载日志失败: ' + error.message)
  }
}

const handleRefresh = () => {
  // 手动刷新时记录当前位置
  isUserAtBottom.value = checkIfAtBottom()
  loadLogs()
}

// 切换自动刷新状态
const toggleAutoRefresh = () => {
  autoRefresh.value = !autoRefresh.value
  // 切换时也执行一次刷新
  handleRefresh()
  ElMessage.success(autoRefresh.value ? '已开启自动刷新' : '已关闭自动刷新')
}

const handleClear = () => {
  logs.value = []
  ElMessage.success('日志已清空')
}

const handleDownload = () => {
  if (logs.value.length === 0) {
    ElMessage.warning('暂无日志可下载')
    return
  }

  // 生成日志内容
  const content = logs.value.join('\n')
  
  // 生成文件名
  const now = new Date()
  const timestamp = now.toISOString().replace(/[:.]/g, '-').slice(0, -5)
  const filename = `log_${timestamp}.txt`
  
  // 创建 Blob 并下载
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
  
  ElMessage.success('日志已下载')
}

const getLogClass = (log: string) => {
  if (log.includes('ERROR') || log.includes('error') || log.includes('失败')) {
    return 'error'
  }
  if (log.includes('WARN') || log.includes('warn') || log.includes('警告')) {
    return 'warning'
  }
  if (log.includes('Desktop-FRP') || log.includes('Desktop-Web')) {
    return 'info'
  }
  return ''
}

// 监听滚动事件，更新用户位置状态
const handleScroll = () => {
  isUserAtBottom.value = checkIfAtBottom()
}

// 监听自动刷新开关
watch(autoRefresh, (newVal) => {
  if (newVal) {
    // 开启自动刷新
    refreshInterval = window.setInterval(() => {
      isUserAtBottom.value = checkIfAtBottom()
      loadLogs()
    }, 2000)
  } else {
    // 关闭自动刷新
    if (refreshInterval) {
      clearInterval(refreshInterval)
      refreshInterval = null
    }
  }
})

onMounted(() => {
  loadStatus()
  // 首次加载，滚动到底部
  loadLogs(true)
  
  // 监听滚动事件
  const container = logsContainer.value
  if (container) {
    container.addEventListener('scroll', handleScroll)
  }
  
  // 默认开启自动刷新
  if (autoRefresh.value) {
    refreshInterval = window.setInterval(() => {
      isUserAtBottom.value = checkIfAtBottom()
      loadLogs()
    }, 2000)
  }
  statusInterval = window.setInterval(loadStatus, 5000)
})

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
  }
  if (statusInterval) {
    clearInterval(statusInterval)
  }
  
  const container = logsContainer.value
  if (container) {
    container.removeEventListener('scroll', handleScroll)
  }
})
</script>

<style scoped>
.logs-page {
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

.header-left p {
  margin: 5px 0 0;
  color: #909399;
  font-size: 13px;
}

.header-left h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 500;
  color: #333;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.diagnostic-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 1px;
  margin: 20px 24px 0;
  overflow: hidden;
  background: #e4e7ed;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
}

.diagnostic-item {
  min-height: 104px;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  padding: 16px 18px;
  background: #fff;
}

.diagnostic-label {
  color: #909399;
  font-size: 12px;
}

.diagnostic-item strong {
  margin-top: 8px;
  color: #303133;
  font-size: 16px;
}

.diagnostic-item small {
  max-width: 100%;
  overflow: hidden;
  margin-top: 5px;
  color: #909399;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.reconnect-button {
  margin-top: 10px;
}

.log-section-title {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin: 22px 24px 0;
}

.version-section-title {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin: 22px 24px 0;
}

.version-section-title h3 { margin: 0; color: #303133; font-size: 15px; font-weight: 600; }
.version-section-title span { color: #909399; font-size: 12px; }

.client-version-row {
  display: grid;
  grid-template-columns: minmax(180px, 1fr) minmax(190px, 1fr) auto;
  align-items: center;
  gap: 20px;
  margin: 10px 24px 0;
  padding: 15px 18px;
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
}

.client-version-row > div { min-width: 0; display: flex; flex-direction: column; gap: 4px; }
.client-version-row strong { color: #303133; font-size: 13px; }
.client-version-row small { overflow: hidden; color: #909399; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }

.log-section-title h3 {
  margin: 0;
  color: #303133;
  font-size: 15px;
  font-weight: 600;
}

.log-section-title span {
  color: #909399;
  font-size: 12px;
}

.logs-content {
  flex: 1;
  overflow-y: auto;
  padding: 12px 24px 24px;
}

.empty {
  background: white;
  border-radius: 8px;
  text-align: center;
  padding: 60px 20px;
  color: #909399;
  font-size: 14px;
}

.log-lines {
  background: white;
  border-radius: 8px;
  padding: 16px;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.8;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
}

.log-line {
  padding: 4px 8px;
  margin: 2px 0;
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-all;
  color: #333;
  transition: background 0.2s;
}

.log-line:hover {
  background: #f5f7fa;
}

.log-line.error {
  color: #f56c6c;
  background: #fef0f0;
  border-left: 3px solid #f56c6c;
  padding-left: 12px;
}

.log-line.warning {
  color: #e6a23c;
  background: #fdf6ec;
  border-left: 3px solid #e6a23c;
  padding-left: 12px;
}

.log-line.info {
  color: #409eff;
  background: #ecf5ff;
  border-left: 3px solid #409eff;
  padding-left: 12px;
}

/* 滚动条样式 */
.logs-content::-webkit-scrollbar {
  width: 8px;
}

.logs-content::-webkit-scrollbar-track {
  background: transparent;
}

.logs-content::-webkit-scrollbar-thumb {
  background: #dcdfe6;
  border-radius: 4px;
}

.logs-content::-webkit-scrollbar-thumb:hover {
  background: #c0c4cc;
}
</style>
