import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  base: './', // 使用相对路径，适配 Wails 嵌入式文件系统
  build: {
    rollupOptions: {
      external: ['@wailsio/runtime']
    }
  }
})
