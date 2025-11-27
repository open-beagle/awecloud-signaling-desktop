import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface ServiceInfo {
  instance_id: number
  instance_name: string
  agent_name: string
  description: string
  service_port: number      // 远程服务端口
  preferred_port?: number   // 用户偏好的本地端口
  access_type?: string      // 'public', 'private', 'group'
  status?: string           // 'online', 'offline'
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
