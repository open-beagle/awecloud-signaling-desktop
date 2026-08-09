import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface ErrorDetail {
  code: string
  message: string
}

export interface UpdateSnapshot {
  operation_id: string
  phase: string
  progress: number
  error?: ErrorDetail
}

export interface IPCStateData {
  session_id: string
  current_version: string
  health: { status: string }
  update?: UpdateSnapshot
}

export const useUpdateStore = defineStore('update', () => {
  const currentVersion = ref('1.0.0')
  const activeUpdate = ref<UpdateSnapshot | null>(null)
  const healthStatus = ref('healthy')
  const isChecking = ref(false)

  const applyState = (data: IPCStateData) => {
    if (data.current_version) {
      currentVersion.value = data.current_version
    }
    if (data.health) {
      healthStatus.value = data.health.status
    }
    activeUpdate.value = data.update || null
  }

  return {
    currentVersion,
    activeUpdate,
    healthStatus,
    isChecking,
    applyState
  }
})
