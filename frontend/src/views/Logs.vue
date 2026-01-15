<template>
  <Layout>
    <div class="logs-page">
      <!-- 页面头部 -->
      <div class="page-header">
        <div class="header-left">
          <h2>查看日志</h2>
          <el-tag v-if="logs.length > 0">
            共 {{ logs.length }} 条
          </el-tag>
        </div>
        <div class="header-right">
          <el-tooltip :content="autoRefresh ? '自动刷新中（点击关闭）' : '点击开启自动刷新'" placement="bottom">
            <el-button 
              :icon="Refresh" 
              :type="autoRefresh ? 'primary' : 'default'"
              @click="toggleAutoRefresh"
              circle
            />
          </el-tooltip>
          <el-tooltip content="滚动到底部" placement="bottom">
            <el-button 
              :icon="Bottom" 
              @click="scrollToBottom"
              circle
            />
          </el-tooltip>
          <el-tooltip content="清空" placement="bottom">
            <el-button 
              :icon="Delete" 
              @click="handleClear"
              circle
            />
          </el-tooltip>
          <el-tooltip content="下载" placement="bottom">
            <el-button 
              :icon="Download" 
              @click="handleDownload"
              circle
            />
          </el-tooltip>
        </div>
      </div>

      <!-- 日志内容 -->
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
  </Layout>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Delete, Download, Bottom } from '@element-plus/icons-vue'
import Layout from '../components/Layout.vue'
import { GetLogs } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/app'

const logs = ref<string[]>([])
const logsContainer = ref<HTMLElement | null>(null)
const autoRefresh = ref(true)
const isUserAtBottom = ref(true)
let refreshInterval: number | null = null

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
})

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
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

.logs-content {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
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
