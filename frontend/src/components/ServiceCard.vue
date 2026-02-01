<template>
  <el-card class="service-card">
    <template #header>
      <div class="card-header">
        <div class="header-left">
          <span class="service-icon">{{ serviceIcon }}</span>
          <span class="service-index">#{{ index }}</span>
          <span class="service-name">{{ service.instance_name }}</span>
        </div>
        <div class="header-right">
          <el-icon 
            class="favorite-icon" 
            :class="{ 'is-favorite': service.is_favorite }"
            @click="toggleFavorite"
          >
            <StarFilled v-if="service.is_favorite" />
            <Star v-else />
          </el-icon>
        </div>
      </div>
    </template>

    <div class="card-body">
      <div class="info-item">
        <span class="label">Agent:</span>
        <span class="value">{{ service.agent_name }}</span>
      </div>
      
      <div class="info-item">
        <span class="label">状态:</span>
        <span class="value">
          <el-tag v-if="service.status === 'online'" type="success" size="small">在线</el-tag>
          <el-tag v-else type="info" size="small">离线</el-tag>
        </span>
      </div>

      <div class="info-item">
        <span class="label">隧道地址:</span>
        <span class="value tunnel-addr" v-if="tunnelAddress">
          {{ tunnelAddress }}
          <el-icon class="copy-icon" @click="copyAddress"><CopyDocument /></el-icon>
        </span>
        <span class="value" v-else>-</span>
      </div>

      <div class="info-item">
        <span class="label">远程服务:</span>
        <span class="value" v-if="service.target_addr">{{ service.target_addr }}</span>
        <span class="value" v-else>-</span>
      </div>

    </div>

    <div class="card-footer">
      <el-button 
        type="primary" 
        size="small" 
        :disabled="service.status !== 'online'"
        @click="handleConnect"
      >
        🔗 连接
      </el-button>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CopyDocument, Star, StarFilled } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { ServiceInfo } from '../stores/services'
import { ToggleFavorite, ConnectService } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/app'

interface Props {
  service: ServiceInfo
  index: number
}

const props = defineProps<Props>()
const emit = defineEmits<{
  favoriteChanged: []
}>()

// 计算隧道地址
const tunnelAddress = computed(() => {
  if (props.service.agent_tailscale_ip && props.service.listen_port) {
    return `${props.service.agent_tailscale_ip}:${props.service.listen_port}`
  }
  return ''
})

// 根据服务名称判断服务类型并返回对应图标
const serviceIcon = computed(() => {
  const name = props.service.instance_name.toLowerCase()
  
  if (name.includes('ssh')) return '🔒'
  if (name.includes('mysql')) return '🗄️'
  if (name.includes('redis')) return '📦'
  if (name.includes('postgres') || name.includes('pg')) return '🐘'
  if (name.includes('mongo')) return '🍃'
  if (name.includes('http') || name.includes('web')) return '🌐'
  if (name.includes('grafana')) return '📊'
  if (name.includes('kibana')) return '🔍'
  if (name.includes('elasticsearch') || name.includes('es')) return '🔎'
  if (name.includes('kafka')) return '📨'
  if (name.includes('rabbitmq') || name.includes('mq')) return '🐰'
  if (name.includes('nginx')) return '🔧'
  if (name.includes('docker')) return '🐳'
  if (name.includes('k8s') || name.includes('kubernetes')) return '☸️'
  
  return '📡' // 默认图标
})

const copyAddress = async () => {
  if (tunnelAddress.value) {
    try {
      await navigator.clipboard.writeText(tunnelAddress.value)
      ElMessage.success('已复制到剪贴板')
    } catch (err) {
      ElMessage.error('复制失败')
    }
  }
}

const toggleFavorite = async () => {
  if (!props.service.service_id) {
    ElMessage.error('服务 ID 不存在')
    return
  }

  try {
    const isFavorite = await ToggleFavorite(props.service.service_id)
    props.service.is_favorite = isFavorite
    ElMessage.success(isFavorite ? '已添加收藏' : '已取消收藏')
    emit('favoriteChanged')
  } catch (err: any) {
    ElMessage.error(err.message || '操作失败')
  }
}

const handleConnect = async () => {
  if (!props.service.service_id) {
    ElMessage.error('服务 ID 不存在')
    return
  }

  if (props.service.status !== 'online') {
    ElMessage.warning('服务离线，无法连接')
    return
  }

  try {
    const command = await ConnectService(props.service.service_id)
    
    // 判断是否是 URL（HTTP/HTTPS）
    if (command.startsWith('http://') || command.startsWith('https://')) {
      // 显示确认对话框
      await ElMessageBox.confirm(
        `是否在浏览器中打开？\n${command}`,
        '打开服务',
        {
          confirmButtonText: '打开',
          cancelButtonText: '取消',
          type: 'info',
        }
      )
      // 在浏览器中打开
      window.open(command, '_blank')
      ElMessage.success('已在浏览器中打开')
    } else {
      // 复制命令到剪贴板
      await navigator.clipboard.writeText(command)
      ElMessage.success({
        message: '连接命令已复制到剪贴板',
        duration: 3000,
        showClose: true,
      })
      
      // 显示命令内容
      ElMessageBox.alert(command, '连接命令', {
        confirmButtonText: '确定',
        type: 'success',
      })
    }
  } catch (err: any) {
    if (err !== 'cancel') {
      ElMessage.error(err.message || '连接失败')
    }
  }
}

</script>

<style scoped>
.service-card {
  transition: all 0.3s;
}

.service-card:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-left {
  display: flex;
  align-items: center;
}

.header-right {
  display: flex;
  align-items: center;
}

.service-icon {
  font-size: 20px;
  margin-right: 8px;
}

.service-index {
  font-size: 14px;
  color: #909399;
  font-weight: 500;
  margin-right: 8px;
}

.service-name {
  font-weight: bold;
  font-size: 16px;
  color: #333;
}

.favorite-icon {
  font-size: 20px;
  cursor: pointer;
  color: #c0c4cc;
  transition: all 0.3s;
}

.favorite-icon:hover {
  color: #f7ba2a;
  transform: scale(1.1);
}

.favorite-icon.is-favorite {
  color: #f7ba2a;
}

.card-body {
  padding: 10px 0;
}

.info-item {
  display: flex;
  justify-content: space-between;
  margin-bottom: 10px;
  font-size: 14px;
}

.info-item:last-child {
  margin-bottom: 0;
}

.info-item .label {
  color: #666;
  font-weight: 500;
}

.info-item .value {
  color: #333;
}

.info-item .value.tunnel-addr {
  color: #409eff;
  font-family: monospace;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 6px;
}

.copy-icon {
  cursor: pointer;
  color: #909399;
  transition: color 0.3s;
}

.copy-icon:hover {
  color: #409eff;
}

.card-footer {
  padding-top: 12px;
  border-top: 1px solid #f0f0f0;
  display: flex;
  justify-content: flex-end;
}
</style>
