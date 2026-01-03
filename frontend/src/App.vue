<template>
  <router-view />
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { GetWindowTitle } from '../bindings/github.com/open-beagle/awecloud-signaling-desktop/app'
import { Window } from '@wailsio/runtime'

// 设置窗口标题
onMounted(async () => {
  try {
    const title = await GetWindowTitle()
    Window.SetTitle(title)
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
