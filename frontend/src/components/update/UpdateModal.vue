<template>
  <el-dialog
    v-model="dialogVisible"
    class="update-dialog"
    :title="`更新到 Beagle Signal ${formatVersion(targetVersion)}`"
    width="560px"
    :close-on-click-modal="!required && !handingOff"
    :close-on-press-escape="!required && !handingOff"
    :show-close="!required && !handingOff"
  >
    <p class="modal-subtitle">确认后由 Launcher 接管升级，当前客户端随后退出。</p>

    <div class="version-flow">
      <div class="version-block">
        <span>当前版本</span><strong>{{ formatVersion(currentVersion) }}</strong>
        <small>{{ shortCommit(currentCommitId) }} · {{ formatTime(currentCommitTime) }}</small>
      </div>
      <el-icon class="version-arrow"><Right /></el-icon>
      <div class="version-block target">
        <span>线上版本</span><strong>{{ formatVersion(targetVersion) }}</strong>
        <small>{{ shortCommit(targetCommitId) }} · {{ formatTime(targetCommitTime) }}</small>
      </div>
    </div>

    <div class="meta">
      <span>安装包 {{ formattedSize }}</span>
      <span>{{ required ? '必须升级' : '稳定通道' }}</span>
    </div>

    <template v-if="handingOff">
      <div class="handoff">
        <el-icon class="is-loading"><Loading /></el-icon>
        <div><strong>正在交给 Launcher</strong><p>Launcher 回复已接管后，Desktop 将立即清理连接并退出。</p></div>
      </div>
    </template>
    <template v-else>
      <section class="release-notes">
        <h3>本次更新</h3>
        <p>{{ releaseNotes || '包含稳定性、安全性与使用体验改进。' }}</p>
      </section>
      <div class="notice">Desktop 不下载更新，也不轮询 Launcher 进度。下载、校验、切换和失败恢复均由 Launcher 完成。</div>
    </template>

    <template #footer>
      <div class="dialog-footer">
        <el-button v-if="!required" :disabled="handingOff" @click="hide">稍后</el-button>
        <el-button v-if="!handingOff" type="primary" :disabled="!targetVersion" @click="$emit('request-update')">更新并重启</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Loading, Right } from '@element-plus/icons-vue'

const props = defineProps<{
  currentVersion: string
  targetVersion?: string
  currentCommitId?: string
  currentCommitTime?: string
  targetCommitId?: string
  targetCommitTime?: string
  releaseNotes?: string
  artifactSize?: number
  required?: boolean
  handingOff?: boolean
}>()

defineEmits(['request-update', 'browser-download'])
const dialogVisible = ref(false)
const formattedSize = computed(() => props.artifactSize ? `${(props.artifactSize / 1024 / 1024).toFixed(1)} MB` : '-')
const formatVersion = (value?: string) => !value ? '-' : value.startsWith('v') ? value : `v${value}`
const shortCommit = (value?: string) => value ? value.slice(0, 8) : '-'
const formatTime = (value?: string) => {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString([], { hour12: false })
}
const show = () => { dialogVisible.value = true }
const hide = () => { if (!props.required && !props.handingOff) dialogVisible.value = false }

defineExpose({ show, hide })
</script>

<style scoped>
.modal-subtitle { margin: -12px 0 18px; color: #6f7c91; font-size: 13px; }
.version-flow { display: grid; grid-template-columns: 1fr 32px 1fr; align-items: center; padding: 16px; background: #f6f8fb; border: 1px solid #e2e7ef; border-radius: 8px; }
.version-block { display: flex; min-width: 0; flex-direction: column; gap: 4px; }
.version-block span, .version-block small { color: #7b8799; font-size: 11px; }
.version-block strong { color: #172033; font-size: 18px; }
.version-block.target strong { color: #365ca8; }
.version-arrow { justify-self: center; color: #8d9bb0; }
.meta { display: flex; gap: 18px; margin: 12px 0 18px; color: #7b8799; font-size: 12px; }
.release-notes { padding: 15px 17px; border: 1px solid #e2e7ef; border-radius: 8px; }
.release-notes h3 { margin: 0 0 8px; color: #172033; font-size: 13px; }
.release-notes p { margin: 0; color: #526075; font-size: 13px; line-height: 1.7; white-space: pre-line; }
.notice { margin-top: 14px; padding: 11px 13px; color: #526075; background: #f3f6fa; border-left: 3px solid #365ca8; font-size: 12px; line-height: 1.6; }
.handoff { display: flex; align-items: center; gap: 14px; padding: 20px; color: #365ca8; background: #eef3fc; border: 1px solid #cbd8ee; border-radius: 8px; }
.handoff .el-icon { flex: 0 0 auto; font-size: 24px; }
.handoff strong { color: #244a91; font-size: 14px; }
.handoff p { margin: 5px 0 0; color: #66758a; font-size: 12px; }
.dialog-footer { display: flex; justify-content: flex-end; gap: 8px; }
</style>
