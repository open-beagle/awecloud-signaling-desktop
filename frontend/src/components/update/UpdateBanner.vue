<template>
  <div v-if="showBanner" class="update-banner" :class="bannerClass">
    <div class="banner-content">
      <el-icon class="banner-icon"><InfoFilled /></el-icon>
      <span class="banner-text">{{ bannerText }}</span>
    </div>
    <div class="banner-actions">
      <el-button v-if="phase === 'staged'" type="primary" size="small" @click="$emit('open-modal')">
        立即重启并安装
      </el-button>
      <el-button v-else-if="phase === 'downloading'" type="info" text size="small" @click="$emit('open-modal')">
        查看进度 ({{ progress }}%)
      </el-button>
      <el-button v-else type="primary" text size="small" @click="$emit('open-modal')">
        查看更新说明
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { InfoFilled } from '@element-plus/icons-vue'

const props = defineProps<{
  phase?: string
  progress?: number
}>()

defineEmits(['open-modal'])

const showBanner = computed(() => {
  return !!props.phase && ['accepted', 'downloading', 'verifying', 'staged'].includes(props.phase)
})

const bannerClass = computed(() => {
  if (props.phase === 'staged') return 'banner-staged'
  return 'banner-info'
})

const bannerText = computed(() => {
  if (props.phase === 'staged') {
    return '新版本已下载就绪，将在下次重启时更新。'
  }
  if (props.phase === 'downloading') {
    return `正在后台下载新版本 (${props.progress || 0}%)...`
  }
  if (props.phase === 'verifying') {
    return '正在验证新版本制品与签名...'
  }
  return '发现新版本可用。'
})
</script>

<style scoped>
.update-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  border-radius: 6px;
  margin-bottom: 12px;
  font-size: 13px;
}

.banner-info {
  background-color: #ecf5ff;
  border: 1px solid #d9ecff;
  color: #409eff;
}

.banner-staged {
  background-color: #f0f9eb;
  border: 1px solid #e1f3d8;
  color: #67c23a;
}

.banner-content {
  display: flex;
  align-items: center;
  gap: 8px;
}

.banner-icon {
  font-size: 16px;
}
</style>
