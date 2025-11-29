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
          <el-tooltip content="刷新" placement="bottom">
            <el-button 
              :icon="Refresh" 
              @click="handleRefresh"
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
      <div class="logs-content">
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
import { ref, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Delete, Download } from '@element-plus/icons-vue'
import Layout from '../components/Layout.vue'
import { GetLogs } from '../../wailsjs/go/main/App'

const logs = ref<string[]>([])
let refreshInterval: number | null = null

const loadLogs = async () => {
  try {
    const result = await GetLogs()
    logs.value = result || []
    
    // 自动滚动到底部
    setTimeout(() => {
      const container = document.querySelector('.logs-content')
      if (container) {
        container.scrollTop = container.scrollHeight
      }
    }, 100)
  } catch (error: any) {
    ElMessage.error('加载日志失败: ' + error.message)
  }
}

const handleRefresh = () => {
  loadLogs()
  ElMessage.success('日志已刷新')
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

onMounted(() => {
  loadLogs()
  
  // 每 2 秒自动刷新
  refreshInterval = window.setInterval(() => {
    loadLogs()
  }, 2000)
})

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
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
