<template>
  <el-tag :type="tagType" size="small">
    {{ statusText }}
  </el-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  status: 'disconnected' | 'connecting' | 'connected' | 'error'
}

const props = defineProps<Props>()

const tagType = computed(() => {
  switch (props.status) {
    case 'connected':
      return 'success'
    case 'connecting':
      return 'warning'
    case 'error':
      return 'danger'
    default:
      return 'info'
  }
})

const statusText = computed(() => {
  switch (props.status) {
    case 'connected':
      return '已连接'
    case 'connecting':
      return '连接中'
    case 'error':
      return '错误'
    default:
      return '未连接'
  }
})
</script>
