<template>
  <Layout>
    <div class="devices-page">
      <!-- 页面头部 -->
      <div class="page-header">
        <div class="header-left">
          <h2>设备管理</h2>
          <el-tag v-if="devices.length > 0">
            共 {{ devices.length }} 台设备
          </el-tag>
        </div>
        <div class="header-right">
          <el-button :icon="Refresh" @click="loadDevices" :loading="loading">
            刷新
          </el-button>
        </div>
      </div>

      <!-- 内容区域 -->
      <div class="devices-content">
        <el-alert
          type="info"
          :closable="false"
          show-icon
          class="info-alert"
        >
          <template #title>
            关于设备管理
          </template>
          <p>这里显示所有使用您的账号登录的设备。您可以让设备下线或删除设备记录。</p>
        </el-alert>

        <el-table
          v-loading="loading"
          :data="devices"
          stripe
          class="devices-table"
        >
      <el-table-column label="设备信息" min-width="200">
        <template #default="{ row }">
          <div class="device-info">
            <div class="device-name">
              <el-icon v-if="row.is_current" color="#67C23A">
                <Check />
              </el-icon>
              {{ row.device_name || '未命名设备' }}
              <el-tag v-if="row.is_current" type="success" size="small">
                当前设备
              </el-tag>
            </div>
            <div class="device-details">
              <span>{{ row.os }} {{ row.arch }}</span>
            </div>
          </div>
        </template>
      </el-table-column>

      <el-table-column label="主机名" prop="hostname" width="150" />

      <el-table-column label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 'online' ? 'success' : 'info'" size="small">
            {{ row.status === 'online' ? '在线' : '离线' }}
          </el-tag>
        </template>
      </el-table-column>

      <el-table-column label="最后使用" width="150">
        <template #default="{ row }">
          <span class="time-text">{{ formatTime(row.last_used_at) }}</span>
        </template>
      </el-table-column>

      <el-table-column label="创建时间" width="150">
        <template #default="{ row }">
          <span class="time-text">{{ formatTime(row.created_at) }}</span>
        </template>
      </el-table-column>

      <el-table-column label="操作" width="200" fixed="right">
        <template #default="{ row }">
          <div class="action-buttons">
            <el-button
              v-if="row.status === 'online' && !row.is_current"
              size="small"
              type="warning"
              @click="handleOffline(row)"
            >
              下线
            </el-button>
            <el-button
              v-if="!row.is_current"
              size="small"
              type="danger"
              @click="handleDelete(row)"
            >
              删除
            </el-button>
            <el-tag v-if="row.is_current" type="success" size="small">
              当前设备
            </el-tag>
          </div>
        </template>
      </el-table-column>
    </el-table>

        <div v-if="devices.length === 0 && !loading" class="empty-state">
          <el-empty description="暂无设备记录" />
        </div>
      </div>
    </div>
  </Layout>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Check } from '@element-plus/icons-vue'
import Layout from '../components/Layout.vue'
import { GetDevices, OfflineDevice, DeleteDevice } from '../../wailsjs/go/main/App'

interface Device {
  device_token: string
  device_name: string
  os: string
  arch: string
  hostname: string
  status: string
  last_used_at: string
  created_at: string
  is_current: boolean
}

const loading = ref(false)
const devices = ref<Device[]>([])

const loadDevices = async () => {
  loading.value = true
  try {
    const result = await GetDevices()
    devices.value = result || []
  } catch (error: any) {
    ElMessage.error('加载设备列表失败: ' + (error.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

const handleOffline = async (device: Device) => {
  try {
    await ElMessageBox.confirm(
      `确定要让设备 "${device.device_name || device.hostname}" 下线吗？该设备将需要重新登录。`,
      '确认下线',
      {
        type: 'warning',
        confirmButtonText: '确定',
        cancelButtonText: '取消'
      }
    )

    await OfflineDevice(device.device_token)
    ElMessage.success('设备已下线')
    loadDevices()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error('操作失败: ' + (error.message || '未知错误'))
    }
  }
}

const handleDelete = async (device: Device) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除设备 "${device.device_name || device.hostname}" 吗？此操作不可恢复。`,
      '确认删除',
      {
        type: 'warning',
        confirmButtonText: '确定',
        cancelButtonText: '取消'
      }
    )

    await DeleteDevice(device.device_token)
    ElMessage.success('设备已删除')
    loadDevices()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error('操作失败: ' + (error.message || '未知错误'))
    }
  }
}

const formatTime = (timeStr: string) => {
  if (!timeStr) return '-'
  const date = new Date(timeStr)
  const now = new Date()
  const diff = now.getTime() - date.getTime()
  const seconds = Math.floor(diff / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)

  if (days > 0) return `${days}天前`
  if (hours > 0) return `${hours}小时前`
  if (minutes > 0) return `${minutes}分钟前`
  return '刚刚'
}

onMounted(() => {
  loadDevices()
})
</script>

<style scoped>
.devices-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: #f5f5f5;
}

.page-header {
  background: white;
  padding: 20px 30px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-left h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 500;
  color: #333;
}

.header-right {
  display: flex;
  gap: 10px;
}

.devices-content {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
}

.info-alert {
  margin-bottom: 24px;
}

.devices-table {
  background: white;
  border-radius: 8px;
  overflow: hidden;
}

.device-info {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.device-name {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 500;
  font-size: 14px;
}

.device-details {
  font-size: 12px;
  color: #666;
}

.time-text {
  font-size: 13px;
  color: #666;
}

.empty-state {
  padding: 40px 0;
  text-align: center;
}

.action-buttons {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: nowrap;
}
</style>
