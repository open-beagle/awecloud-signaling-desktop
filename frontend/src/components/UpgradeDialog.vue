<template>
  <el-dialog
    v-model="visible"
    title="需要升级"
    width="450px"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="false"
  >
    <div class="upgrade-content">
      <div class="warning-icon">
        <el-icon :size="60" color="#E6A23C">
          <WarningFilled />
        </el-icon>
      </div>
      
      <p class="upgrade-message">您的版本过低，需要升级</p>
      
      <div class="version-info">
        <div class="version-item">
          <span class="version-label">当前版本：</span>
          <span class="version-value">{{ currentVersion }}</span>
        </div>
        <div class="version-item">
          <span class="version-label">最新版本：</span>
          <span class="version-value">{{ minVersion }}</span>
        </div>
      </div>
      
      <p class="upgrade-tip">为了您的安全和更好的体验，请升级到最新版本。</p>
    </div>
    
    <template #footer>
      <el-button type="primary" @click="handleDownload" size="large" style="width: 100%">
        立即下载
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { WarningFilled } from '@element-plus/icons-vue'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

const visible = ref(false)
const currentVersion = ref('')
const minVersion = ref('')
const downloadURL = ref('')

const show = (current: string, min: string, url: string) => {
  currentVersion.value = current
  minVersion.value = min
  downloadURL.value = url
  visible.value = true
}

const handleDownload = () => {
  if (!downloadURL.value) {
    return
  }
  
  // 直接打开下载页面 URL
  // URL 已经在 Login.vue 中构建为 服务器地址 + /download
  BrowserOpenURL(downloadURL.value)
}

defineExpose({
  show
})
</script>

<style scoped>
.upgrade-content {
  text-align: center;
  padding: 20px 0;
}

.warning-icon {
  margin-bottom: 20px;
}

.upgrade-message {
  font-size: 18px;
  font-weight: 500;
  color: #333;
  margin: 0 0 20px 0;
}

.version-info {
  background: #f5f7fa;
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 20px;
}

.version-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 0;
}

.version-item:not(:last-child) {
  border-bottom: 1px solid #e4e7ed;
}

.version-label {
  color: #606266;
  font-size: 14px;
}

.version-value {
  color: #303133;
  font-size: 16px;
  font-weight: 500;
  font-family: Consolas, Monaco, 'Courier New', monospace;
}

.upgrade-tip {
  color: #909399;
  font-size: 14px;
  line-height: 1.6;
  margin: 0;
}
</style>
