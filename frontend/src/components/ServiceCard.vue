<template>
  <el-card class="service-card" :class="{ 'connected': isConnected }">
    <template #header>
      <div class="card-header">
        <div class="header-left">
          <span class="service-name">{{ service.instance_name }}</span>
          <el-tag
            v-if="service.access_type"
            :type="getAccessTypeColor(service.access_type)"
            size="small"
            style="margin-left: 8px"
          >
            {{ getAccessTypeLabel(service.access_type) }}
          </el-tag>
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
          <StatusBadge :status="connection.status" />
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
        <span class="label">服务地址:</span>
        <span class="value">{{ service.service_ip }}:{{ service.service_port }}</span>
      </div>

      <div class="info-item" v-if="service.description">
        <span class="label">描述:</span>
        <span class="value">{{ service.description }}</span>
      </div>

      <div class="info-item" v-if="isConnected">
        <span class="label">本地端口:</span>
        <span class="value highlight">{{ connection.local_port }}</span>
      </div>

      <div class="info-item error" v-if="connection.status === 'error'">
        <span class="label">错误:</span>
        <span class="value">{{ connection.error }}</span>
      </div>
    </div>

    <template #footer>
      <div class="card-footer">
        <template v-if="!isConnected && connection.status !== 'connecting'">
          <el-input
            v-model.number="localPort"
            placeholder="本地端口"
            type="number"
            style="width: 120px; margin-right: 10px"
          />
          <el-button
            type="primary"
            @click="handleConnect"
            :disabled="!localPort || localPort < 1 || localPort > 65535 || service.status !== 'online'"
          >
            {{ service.status === 'online' ? '连接' : '服务离线' }}
          </el-button>
        </template>

        <el-button
          v-else-if="connection.status === 'connecting'"
          type="info"
          loading
          disabled
        >
          连接中...
        </el-button>

        <el-button
          v-else
          type="danger"
          @click="handleDisconnect"
        >
          断开连接
        </el-button>
      </div>
    </template>
  </el-card>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Star, StarFilled } from '@element-plus/icons-vue'
import type { ServiceInfo, ConnectionStatus } from '../stores/services'
import { useServicesStore } from '../stores/services'
import StatusBadge from './StatusBadge.vue'
import { ToggleFavorite } from '../../wailsjs/go/main/App'

interface Props {
  service: ServiceInfo
  connection: ConnectionStatus
}

const props = defineProps<Props>()
const emit = defineEmits<{
  connect: [instanceId: number, localPort: number]
  disconnect: [instanceId: number]
}>()

const servicesStore = useServicesStore()

// 使用偏好端口，如果没有则使用服务端口
const localPort = ref(props.service.preferred_port || props.service.service_port)

const isConnected = computed(() => props.connection.status === 'connected')

const getAccessTypeLabel = (type: string) => {
  const labels: Record<string, string> = {
    'public': 'Public',
    'private': 'Private',
    'group': 'Group'
  }
  return labels[type] || 'Public'
}

const getAccessTypeColor = (type: string) => {
  const colors: Record<string, any> = {
    'public': 'success',
    'private': 'warning',
    'group': 'info'
  }
  return colors[type] || 'success'
}

const handleConnect = () => {
  if (localPort.value && localPort.value > 0 && localPort.value <= 65535) {
    emit('connect', props.service.instance_id, localPort.value)
  }
}

const handleDisconnect = () => {
  emit('disconnect', props.service.instance_id)
}

const handleToggleFavorite = async () => {
  // 先乐观更新UI
  servicesStore.toggleFavorite(props.service.instance_id)
  
  // 调用后端API，传递当前端口
  try {
    await ToggleFavorite(props.service.instance_id, localPort.value)
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

.service-card.connected {
  border-color: #67c23a;
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
}

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
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

.info-item .value.highlight {
  color: #67c23a;
  font-weight: bold;
}

.info-item.error .value {
  color: #f56c6c;
  font-size: 12px;
}

.card-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
}
</style>
