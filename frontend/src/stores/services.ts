import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface ServiceInfo {
  instance_id: number
  instance_name: string
  agent_name: string
  description: string
  service_port: number      // Agent端的本地服务端口
  service_ip: string        // Agent端的本地服务IP
  preferred_port?: number   // 用户偏好的本地端口
  status?: string           // 'online', 'offline'
  is_favorite: boolean      // 是否收藏
  service_id?: string       // 服务唯一标识
  // Tailscale 模式字段
  agent_tailscale_ip?: string  // Agent 的 Tailscale IP
  listen_port?: number         // Agent 监听端口
  target_addr?: string         // 内网目标地址
}

export interface ConnectionStatus {
  instance_id: number
  status: 'disconnected' | 'connecting' | 'connected' | 'error'
  local_port: number
  error?: string
}

export const useServicesStore = defineStore('services', () => {
  const services = ref<ServiceInfo[]>([])
  const connections = ref<Map<number, ConnectionStatus>>(new Map())
  const loading = ref(false)

  function setServices(newServices: ServiceInfo[]) {
    services.value = newServices
  }

  function setLoading(value: boolean) {
    loading.value = value
  }

  function updateConnectionStatus(instanceId: number, status: ConnectionStatus) {
    connections.value.set(instanceId, status)
  }

  function getConnectionStatus(instanceId: number): ConnectionStatus {
    return connections.value.get(instanceId) || {
      instance_id: instanceId,
      status: 'disconnected',
      local_port: 0
    }
  }

  function clearConnections() {
    connections.value.clear()
  }

  return {
    services,
    connections,
    loading,
    setServices,
    setLoading,
    updateConnectionStatus,
    getConnectionStatus,
    clearConnections
  }
})
