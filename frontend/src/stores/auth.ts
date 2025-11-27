import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useAuthStore = defineStore('auth', () => {
  const isAuthenticated = ref(false)
  const serverAddress = ref('localhost:8081')
  const clientId = ref('')

  function setAuthenticated(value: boolean) {
    isAuthenticated.value = value
  }

  function setServerAddress(value: string) {
    serverAddress.value = value
  }

  function setClientId(value: string) {
    clientId.value = value
  }

  function logout() {
    isAuthenticated.value = false
    clientId.value = ''
  }

  return {
    isAuthenticated,
    serverAddress,
    clientId,
    setAuthenticated,
    setServerAddress,
    setClientId,
    logout
  }
})
