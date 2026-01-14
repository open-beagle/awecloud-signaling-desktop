<template>
  <el-card class="service-card">
    <template #header>
      <div class="card-header">
        <span class="service-index">#{{ index }}</span>
        <span class="service-name">{{ service.instance_name }}</span>
      </div>
    </template>

    <div class="card-body">
      <div class="info-item">
        <span class="label">Agent:</span>
        <span class="value">{{ service.agent_name }}</span>
      </div>
      
      <div class="info-item">
        <span class="label">状态:</span>
        <span class="value">
          <el-tag v-if="service.status === 'online'" type="success" size="small">在线</el-tag>
          <el-tag v-else type="info" size="small">离线</el-tag>
        </span>
      </div>

      <div class="info-item">
        <span class="label">隧道地址:</span>
        <span class="value tunnel-addr" v-if="tunnelAddress">
          {{ tunnelAddress }}
          <el-icon class="copy-icon" @click="copyAddress"><CopyDocument /></el-icon>
        </span>
        <span class="value" v-else>-</span>
      </div>

      <div class="info-item">
        <span class="label">远程服务:</span>
        <span class="value" v-if="service.target_addr">{{ service.target_addr }}</span>
        <span class="value" v-else>-</span>
      </div>

    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CopyDocument } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import type { ServiceInfo } from '../stores/services'

interface Props {
  service: ServiceInfo
  index: number
}

const props = defineProps<Props>()

// 计算隧道地址
const tunnelAddress = computed(() => {
  if (props.service.agent_tailscale_ip && props.service.listen_port) {
    return `${props.service.agent_tailscale_ip}:${props.service.listen_port}`
  }
  return ''
})

const copyAddress = async () => {
  if (tunnelAddress.value) {
    try {
      await navigator.clipboard.writeText(tunnelAddress.value)
      ElMessage.success('已复制到剪贴板')
    } catch (err) {
      ElMessage.error('复制失败')
    }
  }
}
</script>

<style scoped>
.service-card {
  transition: all 0.3s;
}

.service-card:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.service-index {
  font-size: 14px;
  color: #909399;
  font-weight: 500;
  margin-right: 8px;
}

.service-name {
  font-weight: bold;
  font-size: 16px;
  color: #333;
}

.card-body {
  padding: 10px 0;
}

.info-item {
  display: flex;
  justify-content: space-between;
  margin-bottom: 10px;
  font-size: 14px;
}

.info-item:last-child {
  margin-bottom: 0;
}

.info-item .label {
  color: #666;
  font-weight: 500;
}

.info-item .value {
  color: #333;
}

.info-item .value.tunnel-addr {
  color: #409eff;
  font-family: monospace;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 6px;
}

.copy-icon {
  cursor: pointer;
  color: #909399;
  transition: color 0.3s;
}

.copy-icon:hover {
  color: #409eff;
}
</style>
