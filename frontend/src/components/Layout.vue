<template>
  <div class="app-layout">
    <!-- 顶部导航栏 -->
    <div class="navbar">
      <div class="navbar-left">
        <img src="../assets/logo.png" alt="Logo" class="logo" />
        <span class="app-name">信令桌面</span>
        <span class="server-address" v-if="authStore.serverAddress">{{ authStore.serverAddress }}</span>
      </div>
      
      <div class="navbar-right">
        <!-- 我的服务 -->
        <div 
          class="nav-item"
          :class="{ active: currentRoute === '/services' }"
          @click="navigateTo('/services')"
        >
          <el-icon><Grid /></el-icon>
          <span>我的服务</span>
        </div>
        
        <!-- 我的设备 -->
        <div 
          class="nav-item"
          :class="{ active: currentRoute === '/devices' }"
          @click="navigateTo('/devices')"
        >
          <el-icon><Monitor /></el-icon>
          <span>我的设备</span>
        </div>
        
        <!-- 查看日志 -->
        <div 
          class="nav-item"
          :class="{ active: currentRoute === '/logs' }"
          @click="navigateTo('/logs')"
        >
          <el-icon><Document /></el-icon>
          <span>查看日志</span>
        </div>
        
        <!-- 分隔线 -->
        <div class="nav-divider"></div>
        
        <!-- 用户菜单 -->
        <el-dropdown trigger="hover" @command="handleUserCommand">
          <div class="user-menu">
            <el-icon class="user-icon"><User /></el-icon>
          </div>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item disabled>
                <div class="user-info">
                  <el-icon><User /></el-icon>
                  <span>{{ authStore.clientId }}</span>
                </div>
              </el-dropdown-item>
              <el-dropdown-item divided command="logout">
                <el-icon><SwitchButton /></el-icon>
                <span>注销</span>
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </div>
    
    <!-- 主内容区域 -->
    <div class="main-content">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Grid, Document, Monitor, User, SwitchButton } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { useServicesStore } from '../stores/services'
import { Logout } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/app'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const servicesStore = useServicesStore()

const currentRoute = computed(() => route.path)

const navigateTo = (path: string) => {
  if (currentRoute.value !== path) {
    router.push(path)
  }
}

const handleUserCommand = (command: string) => {
  if (command === 'logout') {
    Logout()
    authStore.logout()
    servicesStore.clearConnections()
    router.push('/login')
    ElMessage.success('已退出登录')
  }
}
</script>

<style scoped>
.app-layout {
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: #f5f5f5;
}

.navbar {
  background: white;
  height: 60px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 24px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  z-index: 100;
}

.navbar-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo {
  width: 32px;
  height: 32px;
  object-fit: contain;
}

.app-name {
  font-size: 16px;
  font-weight: 500;
  color: #333;
}

.server-address {
  font-size: 13px;
  color: #909399;
  padding-left: 12px;
  border-left: 1px solid #e4e7ed;
}

.navbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s;
  color: #666;
  font-size: 14px;
  user-select: none;
}

.nav-item:hover {
  background: #f5f5f5;
  color: #409eff;
}

.nav-item.active {
  background: #e6f4ff;
  color: #409eff;
  font-weight: 500;
}

.nav-item .el-icon {
  font-size: 18px;
}

.nav-divider {
  width: 1px;
  height: 24px;
  background: #e4e7ed;
  margin: 0 8px;
}

.user-menu {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 50%;
  cursor: pointer;
  transition: all 0.2s;
  color: #666;
}

.user-menu:hover {
  background: #f5f5f5;
  color: #409eff;
}

.user-icon {
  font-size: 20px;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
  color: #333;
  font-weight: 500;
}

.main-content {
  flex: 1;
  overflow: hidden;
}
</style>
