<template>
  <router-view />
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { GetWindowTitle } from '../wailsjs/go/main/App'
import { WindowSetTitle } from '../wailsjs/runtime/runtime'

// 设置窗口标题
onMounted(async () => {
  try {
    const title = await GetWindowTitle()
    WindowSetTitle(title)
  } catch (error) {
    console.error('Failed to set window title:', error)
  }
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
