<template>
  <router-view />
</template>

<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { GetWindowTitle } from '../bindings/github.com/open-beagle/awecloud-signaling-desktop/app'
import { Window, Events } from '@wailsio/runtime'
import { ElMessageBox } from 'element-plus'

// 设置窗口标题
onMounted(async () => {
  try {
    const title = await GetWindowTitle()
    Window.SetTitle(title)
  } catch (error) {
    console.error('Failed to set window title:', error)
  }

  // 监听用户禁用事件
  Events.On('auth:disabled', (data: any) => {
    console.log('[App] Received auth:disabled event:', data)
    
    // 弹窗提示用户
    ElMessageBox.alert(
      data.message || '您的账号已被禁用',
      '账号已禁用',
      {
        confirmButtonText: '确定',
        type: 'error',
        showClose: false,
        closeOnClickModal: false,
        closeOnPressEscape: false,
        callback: () => {
          // 用户点击确定后，跳转到登录页
          window.location.href = '/#/login'
        }
      }
    )
  })
})

onUnmounted(() => {
  // 清理事件监听
  Events.Off('auth:disabled')
})
</script>

<style>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

#app {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}
</style>
