<template>
  <div class="logs-container">
    <!-- 顶部栏 -->
    <div class="header">
      <div class="header-left">
        <h2>系统日志</h2>
      </div>
      <div class="header-right">
        <el-button :icon="Refresh" @click="handleRefresh">
          刷新
        </el-button>
        <el-button @click="handleClear">
          清空
        </el-button>
        <el-button @click="handleBack">
          返回
        </el-button>
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
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { GetLogs } from '../../wailsjs/go/main/App'

const router = useRouter()
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

const handleBack = () => {
  router.back()
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
.logs-container {
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: #1e1e1e;
  color: #d4d4d4;
}

.header {
  background: #2d2d2d;
  padding: 15px 20px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid #3e3e3e;
}

.header-left h2 {
  margin: 0;
  font-size: 18px;
  color: #d4d4d4;
}

.header-right {
  display: flex;
  gap: 10px;
}

.logs-content {
  flex: 1;
  overflow-y: auto;
  padding: 10px;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.6;
}

.empty {
  text-align: center;
  padding: 40px;
  color: #666;
}

.log-lines {
  display: flex;
  flex-direction: column;
}

.log-line {
  padding: 2px 5px;
  white-space: pre-wrap;
  word-break: break-all;
}

.log-line.error {
  color: #f48771;
  background: rgba(244, 135, 113, 0.1);
}

.log-line.warning {
  color: #dcdcaa;
  background: rgba(220, 220, 170, 0.1);
}

.log-line.info {
  color: #4ec9b0;
}

/* 滚动条样式 */
.logs-content::-webkit-scrollbar {
  width: 10px;
}

.logs-content::-webkit-scrollbar-track {
  background: #1e1e1e;
}

.logs-content::-webkit-scrollbar-thumb {
  background: #424242;
  border-radius: 5px;
}

.logs-content::-webkit-scrollbar-thumb:hover {
  background: #4e4e4e;
}
</style>
