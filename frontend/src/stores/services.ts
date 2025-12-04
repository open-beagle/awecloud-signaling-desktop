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
  access_type?: string      // 'public', 'private', 'group'
  status?: string           // 'online', 'offline'
  is_favorite?: boolean     // 是否收藏
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
    // 服务的收藏状态由服务器返回，不需要本地处理
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

  function toggleFavorite(instanceId: number) {
    // 更新对应服务的收藏状态（乐观更新）
    const service = services.value.find(s => s.instance_id === instanceId)
    if (service) {
      service.is_favorite = !service.is_favorite
    }
  }

  function isFavorite(instanceId: number): boolean {
    const service = services.value.find(s => s.instance_id === instanceId)
    return service?.is_favorite || false
  }

  return {
    services,
    connections,
    loading,
    setServices,
    setLoading,
    updateConnectionStatus,
    getConnectionStatus,
    clearConnections,
    toggleFavorite,
    isFavorite
  }
})
