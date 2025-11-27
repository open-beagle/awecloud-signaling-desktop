<template>
  <el-card class="service-card" :class="{ 'connected': isConnected }">
    <template #header>
      <div class="card-header">
        <span class="service-name">{{ service.instance_name }}</span>
        <StatusBadge :status="connection.status" />
      </div>
    </template>

    <div class="card-body">
      <div class="info-item">
        <span class="label">Agent:</span>
        <span class="value">{{ service.agent_name }}</span>
      </div>
      
      <div class="info-item">
        <span class="label">远程端口:</span>
        <span class="value">{{ service.service_port }}</span>
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
            :disabled="!localPort || localPort < 1 || localPort > 65535"
          >
            连接
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
import type { ServiceInfo, ConnectionStatus } from '../stores/services'
import StatusBadge from './StatusBadge.vue'

interface Props {
  service: ServiceInfo
  connection: ConnectionStatus
}

const props = defineProps<Props>()
const emit = defineEmits<{
  connect: [instanceId: number, localPort: number]
  disconnect: [instanceId: number]
}>()

const localPort = ref(props.service.service_port)

const isConnected = computed(() => props.connection.status === 'connected')

const handleConnect = () => {
  if (localPort.value && localPort.value > 0 && localPort.value <= 65535) {
    emit('connect', props.service.instance_id, localPort.value)
  }
}

const handleDisconnect = () => {
  emit('disconnect', props.service.instance_id)
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
