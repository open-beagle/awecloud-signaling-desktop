<template>
  <el-card class="service-card">
    <template #header>
      <div class="card-header">
        <div class="header-left">
          <span class="service-index">#{{ index }}</span>
          <span class="service-name">{{ service.instance_name }}</span>
        </div>
        <div class="header-right">
          <el-icon 
            class="favorite-icon" 
            :class="{ 'is-favorite': service.is_favorite }"
            @click="handleToggleFavorite"
          >
            <StarFilled v-if="service.is_favorite" />
            <Star v-else />
          </el-icon>
        </div>
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

      <div class="info-item" v-if="service.target_addr">
        <span class="label">目标地址:</span>
        <span class="value">{{ service.target_addr }}</span>
      </div>

      <div class="info-item" v-if="service.description">
        <span class="label">描述:</span>
        <span class="value">{{ service.description }}</span>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Star, StarFilled, CopyDocument } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import type { ServiceInfo } from '../stores/services'
import { useServicesStore } from '../stores/services'
import { ToggleFavorite } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/app'

interface Props {
  service: ServiceInfo
  index: number
}

const props = defineProps<Props>()

const servicesStore = useServicesStore()

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

const handleToggleFavorite = async () => {
  // 先乐观更新UI
  servicesStore.toggleFavorite(props.service.instance_id)
  
  // 调用后端API
  try {
    await ToggleFavorite(props.service.instance_id, 0)
  } catch (error: any) {
    // 如果失败，回滚UI状态
    servicesStore.toggleFavorite(props.service.instance_id)
    console.error('Failed to toggle favorite:', error)
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

.header-left {
  display: flex;
  align-items: center;
  flex: 1;
  gap: 8px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.service-index {
  font-size: 14px;
  color: #909399;
  font-weight: 500;
  min-width: 30px;
}

.service-name {
  font-weight: bold;
  font-size: 16px;
  color: #333;
}

.favorite-icon {
  font-size: 20px;
  cursor: pointer;
  color: #dcdfe6;
  transition: all 0.3s;
}

.favorite-icon:hover {
  color: #ffd700;
  transform: scale(1.2);
}

.favorite-icon.is-favorite {
  color: #ffd700;
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
