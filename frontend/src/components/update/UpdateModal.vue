<template>
  <el-dialog
    v-model="dialogVisible"
    title="软件更新"
    width="480px"
    :close-on-click-modal="false"
    :destroy-on-close="true"
  >
    <div class="update-dialog-body">
      <div class="version-header">
        <div class="version-badge">
          <span class="label">当前版本</span>
          <span class="val">{{ currentVersion }}</span>
        </div>
        <el-icon class="arrow-icon"><Right /></el-icon>
        <div class="version-badge target">
          <span class="label">目标版本</span>
          <span class="val">{{ targetVersion }}</span>
        </div>
      </div>

      <div v-if="phase === 'downloading' || phase === 'verifying'" class="progress-section">
        <div class="progress-title">
          <span>{{ phase === 'downloading' ? '正在后台下载...' : '正在校验制品...' }}</span>
          <span>{{ progress }}%</span>
        </div>
        <el-progress :percentage="progress" :status="phase === 'verifying' ? 'success' : ''" />
      </div>

      <div class="release-notes-box">
        <div class="notes-title">更新说明</div>
        <div class="notes-content">
          <p>{{ releaseNotes || '优化系统稳定性与安全防护能力。' }}</p>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="dialogVisible = false">稍后再说</el-button>
        <el-button v-if="phase === 'staged'" type="success" :loading="submitting" @click="handleConfirm">
          重启并安装
        </el-button>
        <el-button v-else-if="!phase || phase === 'failed'" type="primary" :loading="submitting" @click="handleRequest">
          立即更新
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Right } from '@element-plus/icons-vue'

const props = defineProps<{
  currentVersion: string
  targetVersion?: string
  releaseNotes?: string
  phase?: string
  progress?: number
  operationId?: string
}>()

const emit = defineEmits(['request-update', 'confirm-update'])

const dialogVisible = ref(false)
const submitting = ref(false)

const show = () => {
  dialogVisible.value = true
}

const hide = () => {
  dialogVisible.value = false
}

const handleRequest = async () => {
  submitting.value = true
  try {
    emit('request-update')
  } finally {
    submitting.value = false
  }
}

const handleConfirm = async () => {
  submitting.value = true
  try {
    emit('confirm-update', props.operationId)
    hide()
  } finally {
    submitting.value = false
  }
}

defineExpose({ show, hide })
</script>

<style scoped>
.update-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.version-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background-color: #f5f7fa;
  padding: 14px 20px;
  border-radius: 8px;
}

.version-badge {
  display: flex;
  flex-direction: column;
}

.version-badge .label {
  font-size: 11px;
  color: #909399;
}

.version-badge .val {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}

.version-badge.target .val {
  color: #409eff;
}

.arrow-icon {
  font-size: 18px;
  color: #909399;
}

.progress-section {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.progress-title {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  color: #606266;
}

.release-notes-box {
  border: 1px solid #e4e7ed;
  border-radius: 6px;
  padding: 12px;
}

.notes-title {
  font-weight: 600;
  font-size: 13px;
  margin-bottom: 6px;
  color: #303133;
}

.notes-content {
  font-size: 12px;
  color: #606266;
  line-height: 1.6;
}
</style>
