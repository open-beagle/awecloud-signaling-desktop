<template>
  <div class="devices-page">
    <!-- 页面头部 -->
    <div class="page-header">
      <div class="header-left">
        <h2>我的设备与远程桌面</h2>
        <el-tag v-if="myDevices.length > 0" type="primary" effect="plain">
          我的设备 {{ myDevices.length }} 台
        </el-tag>
        <el-tag v-if="sharedDevices.length > 0" type="warning" effect="plain">
          授权给我 {{ sharedDevices.length }} 台
        </el-tag>
      </div>
      <div class="header-right">
        <el-tooltip content="刷新设备与远程桌面状态" placement="bottom">
          <el-button
            :icon="Refresh"
            @click="refreshAll"
            :loading="loading"
            circle
          />
        </el-tooltip>
      </div>
    </div>

    <!-- 内容区域 -->
    <div class="devices-content">
      <!-- 本机远程桌面与无人值守设置卡片 -->
      <el-card shadow="never" class="local-rdp-card">
        <template #header>
          <div class="card-header">
            <div class="card-title-group">
              <el-icon class="card-icon"><Monitor /></el-icon>
              <span class="card-title">本机远程桌面（3389）零信任开放与值守</span>
              <el-tag v-if="currentDevice?.rdp_admin_disabled" type="danger" size="small">
                平台管理员已紧急熔断关闭
              </el-tag>
              <el-tag
                v-else-if="currentDevice?.rdp_tunnel_exposed || (localRDPStatus?.ready && effectiveCurrentMode !== 'disabled')"
                type="success"
                size="small"
              >
                隧道已就绪 · 本机端口 {{ localRDPStatus?.local_port || currentDevice?.rdp_local_port || 3389 }}
              </el-tag>
              <el-tag v-else-if="localRDPStatus?.ready" type="warning" size="small">
                本机已监听 :{{ localRDPStatus.local_port }} · 入站已关闭
              </el-tag>
              <el-tag v-else type="info" size="small">
                {{ localRDPStatusText }}
              </el-tag>
            </div>
            <span v-if="currentDevice" class="current-device-meta">
              当前设备：{{ currentDevice.device_name || currentDevice.hostname }}
              <template v-if="currentDevice.ip"> ({{ currentDevice.ip }})</template>
            </span>
          </div>
        </template>

        <div class="settings-grid">
          <div class="setting-item">
            <div class="setting-label">本机 RDP 开放策略</div>
            <div class="setting-control">
              <el-radio-group
                :model-value="effectiveCurrentMode"
                size="small"
                :disabled="!currentDevice || currentDevice.rdp_admin_disabled || updatingToken === currentDevice.device_token"
                @change="(val: any) => currentDevice && handleModeChange(currentDevice, String(val))"
              >
                <el-radio-button value="auto">自动开放（推荐）</el-radio-button>
                <el-radio-button value="enabled">强制开启</el-radio-button>
                <el-radio-button value="disabled">关闭</el-radio-button>
              </el-radio-group>
            </div>
            <div class="setting-desc">
              仅允许您本人登录的其他 Signal Desktop 访问，严格隔离同账号 CloudIDE 与分组网络，不暴露任何公网端口。
            </div>
          </div>

          <div class="setting-item">
            <div class="setting-label">跨用户临时共享许可</div>
            <div class="setting-control">
              <el-switch
                :model-value="Boolean(currentDevice?.rdp_allow_share)"
                :disabled="!currentDevice || currentDevice.rdp_admin_disabled || updatingToken === currentDevice.device_token"
                active-text="允许租户临时授权"
                inactive-text="仅限本人设备"
                @change="(val: any) => currentDevice && handleAllowShareChange(currentDevice, Boolean(val))"
              />
            </div>
            <div class="setting-desc">
              默认关闭。开启后，租户管理员方可为您创建带到期时间的同租户成员协助授权，关闭即刻收回。
            </div>
          </div>

          <div class="setting-item">
            <div class="setting-label">开机登录自动值守</div>
            <div class="setting-control">
              <el-switch
                :model-value="autostartEnabled"
                :loading="autostartSaving"
                active-text="已开启登录自启"
                inactive-text="未开启"
                @change="(val: any) => handleAutostartChange(Boolean(val))"
              />
            </div>
            <div class="setting-desc">
              出差前开启后，系统重启登录时自动后台启动 Beagle Signal 并恢复零信任隧道连接。
            </div>
          </div>
        </div>

        <div v-if="showWindowsUnattendedTip" class="unattended-tip">
          <span>
            <strong>Windows 出差无人值守提示：</strong>
            请在系统“电源和睡眠”中将插电状态设为<strong>从不睡眠</strong>；保持系统级网络级别身份验证（NLA）开启；若平时使用 Windows Hello PIN 解锁，远程连接时请输入微软账户密码或本地账户密码。
          </span>
        </div>
      </el-card>

      <!-- 我的设备列表 -->
      <div class="section-block">
        <div class="section-header">
          <h3>我的账号设备</h3>
          <span class="section-subtitle">
            在任意一台 Signal Desktop 上均可一键连接家中/办公室电脑，或远程关闭某台设备的 RDP 暴露
          </span>
        </div>

        <el-table
          v-loading="loading"
          :data="myDevices"
          stripe
          class="devices-table"
        >
          <el-table-column label="设备 / 主机名" min-width="200">
            <template #default="{ row }">
              <div class="device-primary">
                <span class="device-title">{{ row.device_name || row.hostname || '-' }}</span>
                <el-tag v-if="row.is_current" type="success" size="small" effect="light">
                  本机
                </el-tag>
              </div>
              <div v-if="row.hostname && row.device_name && row.hostname !== row.device_name" class="device-sub">
                主机名: {{ row.hostname }}
              </div>
            </template>
          </el-table-column>

          <el-table-column label="系统环境" min-width="150">
            <template #default="{ row }">
              <span>{{ formatOSInfo(row.os) }} {{ formatArchInfo(row.arch) }}</span>
            </template>
          </el-table-column>

          <el-table-column label="隧道状态" min-width="175">
            <template #default="{ row }">
              <span v-if="row.status === 'online'" class="status-online">
                ● 在线<span v-if="row.ip" class="status-detail"> · {{ row.ip }}</span>
              </span>
              <span v-else class="status-offline">
                ○ 离线<span v-if="row.last_used_at" class="status-detail"> · {{ formatTime(row.last_used_at) }}</span>
              </span>
            </template>
          </el-table-column>

          <el-table-column label="远程桌面状态" min-width="190">
            <template #default="{ row }">
              <div class="rdp-status-cell">
                <el-tag :type="getRDPBadge(row).type" size="small" effect="plain">
                  {{ getRDPBadge(row).label }}
                </el-tag>
                <el-tag
                  v-if="row.rdp_allow_share && !row.rdp_admin_disabled"
                  type="warning"
                  size="small"
                  effect="plain"
                >
                  可临时共享
                </el-tag>
              </div>
            </template>
          </el-table-column>

          <el-table-column label="开放模式与共享" min-width="235">
            <template #default="{ row }">
              <div class="policy-controls">
                <el-select
                  :model-value="normalizeMode(row.rdp_mode)"
                  size="small"
                  class="mode-select"
                  :disabled="row.rdp_admin_disabled || updatingToken === row.device_token"
                  @change="(val: any) => handleModeChange(row, String(val))"
                >
                  <el-option label="自动开放" value="auto" />
                  <el-option label="强制开启" value="enabled" />
                  <el-option label="关闭暴露" value="disabled" />
                </el-select>
                <el-tooltip content="允许租户管理员为此设备创建带有效期的跨人协助授权" placement="top">
                  <el-switch
                    :model-value="Boolean(row.rdp_allow_share)"
                    size="small"
                    :disabled="row.rdp_admin_disabled || updatingToken === row.device_token"
                    @change="(val: any) => handleAllowShareChange(row, Boolean(val))"
                  />
                </el-tooltip>
              </div>
            </template>
          </el-table-column>

          <el-table-column label="操作" width="260" fixed="right">
            <template #default="{ row }">
              <div class="action-buttons">
                <template v-if="!row.is_current">
                  <el-button
                    size="small"
                    type="primary"
                    :icon="Monitor"
                    :loading="connectingToken === row.device_token"
                    :disabled="!canConnectRDP(row)"
                    @click="handleConnectRDP(row)"
                  >
                    远程桌面
                  </el-button>
                  <el-tooltip content="分配本地无冲突代理端口并复制连接地址" placement="top">
                    <el-button
                      size="small"
                      :icon="CopyDocument"
                      :loading="copyingToken === row.device_token"
                      :disabled="!canConnectRDP(row)"
                      @click="handleCopyRDPEndpoint(row)"
                      circle
                    />
                  </el-tooltip>
                  <el-tooltip v-if="row.status === 'online'" content="强制设备下线" placement="top">
                    <el-button
                      size="small"
                      type="warning"
                      :icon="SwitchButton"
                      @click="handleOffline(row)"
                      circle
                    />
                  </el-tooltip>
                  <el-tooltip content="删除设备记录" placement="top">
                    <el-button
                      size="small"
                      type="danger"
                      :icon="Delete"
                      @click="handleDelete(row)"
                      circle
                    />
                  </el-tooltip>
                </template>
                <span v-else class="current-hint">当前主控/值守终端</span>
              </div>
            </template>
          </el-table-column>
        </el-table>

        <div v-if="myDevices.length === 0 && !loading" class="empty-state">
          <el-empty description="暂无本人设备记录" />
        </div>
      </div>

      <!-- 授权给我的跨用户远程桌面 -->
      <div v-if="sharedDevices.length > 0" class="section-block shared-section">
        <div class="section-header">
          <h3>授权给我的远程桌面</h3>
          <span class="section-subtitle">
            来自同租户成员显式开启共享许可并经管理员签发的临时授权，到期或撤销后连接立即断开
          </span>
        </div>

        <el-table :data="sharedDevices" stripe class="devices-table">
          <el-table-column label="设备归属人" min-width="150">
            <template #default="{ row }">
              <strong>{{ row.owner_username || '-' }}</strong>
            </template>
          </el-table-column>

          <el-table-column label="设备 / 主机名" min-width="190">
            <template #default="{ row }">
              <span>{{ row.device_name || row.hostname || '-' }}</span>
            </template>
          </el-table-column>

          <el-table-column label="系统环境" min-width="140">
            <template #default="{ row }">
              <span>{{ formatOSInfo(row.os) }} {{ formatArchInfo(row.arch) }}</span>
            </template>
          </el-table-column>

          <el-table-column label="在线与 RDP 状态" min-width="190">
            <template #default="{ row }">
              <div class="rdp-status-cell">
                <el-tag :type="row.status === 'online' ? 'success' : 'info'" size="small" effect="plain">
                  {{ row.status === 'online' ? '在线' : '离线' }}
                </el-tag>
                <el-tag :type="getRDPBadge(row).type" size="small" effect="plain">
                  {{ getRDPBadge(row).label }}
                </el-tag>
              </div>
            </template>
          </el-table-column>

          <el-table-column label="授权有效期" min-width="200">
            <template #default="{ row }">
              <span class="expiry-text">{{ formatGrantExpiry(row.grant_expires_at_unix) }}</span>
            </template>
          </el-table-column>

          <el-table-column label="操作" width="190" fixed="right">
            <template #default="{ row }">
              <div class="action-buttons">
                <el-button
                  size="small"
                  type="primary"
                  :icon="Monitor"
                  :loading="connectingToken === row.device_token"
                  :disabled="!canConnectRDP(row)"
                  @click="handleConnectRDP(row)"
                >
                  远程桌面
                </el-button>
                <el-tooltip content="复制本地代理连接地址" placement="top">
                  <el-button
                    size="small"
                    :icon="CopyDocument"
                    :loading="copyingToken === row.device_token"
                    :disabled="!canConnectRDP(row)"
                    @click="handleCopyRDPEndpoint(row)"
                    circle
                  />
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, SwitchButton, Delete, Monitor, CopyDocument } from '@element-plus/icons-vue'
import { Events } from '@wailsio/runtime'
import {
  GetDevices,
  OfflineDevice,
  DeleteDevice,
  GetLocalRDPStatus,
  UpdateDeviceRDPConfig,
  PrepareDeviceRDPEndpoint,
  ConnectDeviceRDP,
  GetAutostartEnabled,
  SetAutostartEnabled,
} from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/internal/app/app'
import type { DeviceInfo } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/internal/app/models'
import type { LocalRDPStatus } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/internal/rdp/models'

const loading = ref(false)
const devices = ref<DeviceInfo[]>([])
const localRDPStatus = ref<LocalRDPStatus | null>(null)
const autostartEnabled = ref(false)
const autostartSaving = ref(false)
const updatingToken = ref('')
const connectingToken = ref('')
const copyingToken = ref('')

const myDevices = computed(() => devices.value.filter(d => !d.is_shared))
const sharedDevices = computed(() => devices.value.filter(d => d.is_shared))
const currentDevice = computed(() => myDevices.value.find(d => d.is_current) || null)

const normalizeMode = (mode?: string) => {
  if (mode === 'enabled' || mode === 'disabled') return mode
  return 'auto'
}

const effectiveCurrentMode = computed(() => normalizeMode(currentDevice.value?.rdp_mode))

const localRDPStatusText = computed(() => {
  if (!localRDPStatus.value) return '正在检测本机 RDP 状态...'
  if (!localRDPStatus.value.supported) return '当前系统不作为 RDP 被控端'
  return localRDPStatus.value.reason || '未检测到本机 3389 远程桌面监听'
})

const showWindowsUnattendedTip = computed(() => {
  const osName = (localRDPStatus.value?.os || currentDevice.value?.os || '').toLowerCase()
  return osName.includes('win')
})

const loadDevices = async () => {
  loading.value = true
  try {
    const result = await GetDevices()
    devices.value = (result || []).filter((d): d is DeviceInfo => d !== null)
  } catch (error: any) {
    ElMessage.error('加载设备列表失败: ' + (error?.message || error || '未知错误'))
  } finally {
    loading.value = false
  }
}

const loadLocalStatus = async () => {
  try {
    const [rdpStatus, autoStart] = await Promise.all([
      GetLocalRDPStatus(),
      GetAutostartEnabled(),
    ])
    if (rdpStatus) {
      localRDPStatus.value = rdpStatus
    }
    autostartEnabled.value = Boolean(autoStart)
  } catch (error) {
    console.warn('读取本机 RDP 或自启动状态失败:', error)
  }
}

const refreshAll = async () => {
  await Promise.all([loadDevices(), loadLocalStatus()])
}

const canConnectRDP = (device: DeviceInfo) => {
  if (device.is_current) return false
  if (device.status !== 'online' || !device.ip) return false
  if (device.rdp_admin_disabled) return false
  if (normalizeMode(device.rdp_mode) === 'disabled') return false
  return Boolean(device.rdp_connectable || device.rdp_tunnel_exposed || device.rdp_local_ready)
}

const getRDPBadge = (device: DeviceInfo): { label: string; type: 'success' | 'warning' | 'danger' | 'info' } => {
  if (device.rdp_admin_disabled) {
    return { label: '平台已熔断关闭', type: 'danger' }
  }
  const mode = normalizeMode(device.rdp_mode)
  if (mode === 'disabled') {
    return { label: '已关闭暴露', type: 'info' }
  }
  if (device.status !== 'online') {
    return { label: device.rdp_local_ready ? '离线 (曾就绪)' : '设备离线', type: 'info' }
  }
  const port = device.rdp_local_port || 3389
  if (device.rdp_tunnel_exposed || device.rdp_connectable) {
    return { label: `可连接 (:${port})`, type: 'success' }
  }
  if (device.rdp_local_ready) {
    return { label: `本机就绪 (:${port})`, type: 'warning' }
  }
  const osLower = (device.os || '').toLowerCase()
  if (osLower.includes('darwin') || osLower.includes('mac')) {
    return { label: '仅作主控端', type: 'info' }
  }
  return { label: '未监听 RDP', type: 'info' }
}

const applyUpdatedDevice = (updated: DeviceInfo | null) => {
  if (!updated) return
  const idx = devices.value.findIndex(d => d.device_token === updated.device_token)
  if (idx >= 0) {
    devices.value[idx] = updated
  } else {
    loadDevices()
  }
}

const handleModeChange = async (device: DeviceInfo, rdpMode: string) => {
  if (updatingToken.value) return
  updatingToken.value = device.device_token
  try {
    const updated = await UpdateDeviceRDPConfig({
      device_token: device.device_token,
      rdp_mode: rdpMode,
      rdp_allow_share: Boolean(device.rdp_allow_share),
    })
    applyUpdatedDevice(updated)
    ElMessage.success(`已将 "${device.device_name || device.hostname}" 的远程桌面策略设为${rdpMode === 'auto' ? '自动开放' : rdpMode === 'enabled' ? '强制开启' : '关闭'}`)
  } catch (error: any) {
    ElMessage.error('更新远程桌面策略失败: ' + (error?.message || error || '未知错误'))
    await loadDevices()
  } finally {
    updatingToken.value = ''
  }
}

const handleAllowShareChange = async (device: DeviceInfo, allowShare: boolean) => {
  if (updatingToken.value) return
  updatingToken.value = device.device_token
  try {
    const updated = await UpdateDeviceRDPConfig({
      device_token: device.device_token,
      rdp_mode: normalizeMode(device.rdp_mode),
      rdp_allow_share: allowShare,
    })
    applyUpdatedDevice(updated)
    ElMessage.success(allowShare ? '已允许租户管理员创建临时协助授权' : '已关闭跨用户共享许可（现有跨人连接即刻失效）')
  } catch (error: any) {
    ElMessage.error('更新共享许可失败: ' + (error?.message || error || '未知错误'))
    await loadDevices()
  } finally {
    updatingToken.value = ''
  }
}

const handleAutostartChange = async (enabled: boolean) => {
  autostartSaving.value = true
  try {
    await SetAutostartEnabled(enabled)
    autostartEnabled.value = enabled
    ElMessage.success(enabled ? '已开启系统登录自动启动' : '已关闭系统登录自动启动')
  } catch (error: any) {
    ElMessage.error('设置开机自启动失败: ' + (error?.message || error || '未知错误'))
  } finally {
    autostartSaving.value = false
  }
}

const copyText = async (text: string) => {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text)
    return
  }
  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  document.execCommand('copy')
  document.body.removeChild(textarea)
}

const handleConnectRDP = async (device: DeviceInfo) => {
  connectingToken.value = device.device_token
  try {
    const result = await ConnectDeviceRDP(device.device_token)
    if (!result) return
    if (result.launched) {
      ElMessage.success(result.message || `已拉起远程桌面连接 (${result.endpoint?.recommended_target || result.endpoint?.local_addr})`)
    } else {
      const target = result.endpoint?.recommended_target || result.endpoint?.local_addr || ''
      if (target) {
        await copyText(target)
      }
      await ElMessageBox.alert(
        `${result.message}\n\n推荐连接地址（已自动复制）：${target}${result.rdp_file_path ? `\n配置文件路径：${result.rdp_file_path}` : ''}`,
        '远程桌面连接信息',
        { confirmButtonText: '知道了', type: 'info' }
      )
    }
  } catch (error: any) {
    ElMessage.error('发起远程桌面失败: ' + (error?.message || error || '未知错误'))
  } finally {
    connectingToken.value = ''
  }
}

const handleCopyRDPEndpoint = async (device: DeviceInfo) => {
  copyingToken.value = device.device_token
  try {
    const endpoint = await PrepareDeviceRDPEndpoint(device.device_token)
    if (!endpoint) return
    const target = endpoint.recommended_target || endpoint.local_addr
    await copyText(target)
    ElMessage.success(`已复制本地连接地址：${target}（VIP 域名：${endpoint.domain}）`)
  } catch (error: any) {
    ElMessage.error('准备连接地址失败: ' + (error?.message || error || '未知错误'))
  } finally {
    copyingToken.value = ''
  }
}

const handleOffline = async (device: DeviceInfo) => {
  try {
    await ElMessageBox.confirm(
      `确定要让设备 "${device.device_name || device.hostname}" 下线吗？该设备将需要重新登录。`,
      '确认下线',
      { type: 'warning', confirmButtonText: '确定', cancelButtonText: '取消' }
    )
    await OfflineDevice(device.device_token)
    ElMessage.success('设备已下线')
    loadDevices()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error('操作失败: ' + (error?.message || '未知错误'))
    }
  }
}

const handleDelete = async (device: DeviceInfo) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除设备 "${device.device_name || device.hostname}" 吗？此操作不可恢复。`,
      '确认删除',
      { type: 'warning', confirmButtonText: '确定', cancelButtonText: '取消' }
    )
    await DeleteDevice(device.device_token)
    ElMessage.success('设备已删除')
    loadDevices()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error('操作失败: ' + (error?.message || '未知错误'))
    }
  }
}

const formatOSInfo = (os: string) => {
  if (!os) return '未知系统'
  if (os.includes('Windows') || os === 'macOS' || os === 'Linux') return os
  switch (os.toLowerCase()) {
    case 'windows': return 'Windows'
    case 'darwin': return 'macOS'
    case 'linux': return 'Linux'
    case 'desktop': return 'Windows'
    default: return os
  }
}

const formatArchInfo = (arch: string) => {
  if (!arch) return ''
  if (arch === 'x86_64' || arch === 'ARM64' || arch === 'x86') return arch
  switch (arch.toLowerCase()) {
    case 'amd64': return 'x86_64'
    case 'arm64': return 'ARM64'
    case '386': return 'x86'
    case 'unknown': return 'x86_64'
    default: return arch
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

const formatGrantExpiry = (unixSec: number) => {
  if (!unixSec) return '-'
  const target = new Date(unixSec * 1000)
  const diffMs = target.getTime() - Date.now()
  if (diffMs <= 0) return '已到期'
  const totalMinutes = Math.ceil(diffMs / 60000)
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  const remain = hours > 0 ? `剩余 ${hours}小时${minutes}分` : `剩余 ${minutes}分钟`
  return `${target.toLocaleString()} (${remain})`
}

onMounted(() => {
  refreshAll()
  Events.On('devices:updated', () => {
    loadDevices()
  })
  Events.On('rdp:local-status', (ev: any) => {
    const payload = ev?.data?.[0] ?? ev?.data ?? ev
    if (payload && typeof payload === 'object' && 'ready' in payload) {
      localRDPStatus.value = payload as LocalRDPStatus
    } else {
      loadLocalStatus()
    }
  })
  Events.On('rdp:status-updated', () => {
    refreshAll()
  })
})

onUnmounted(() => {
  Events.Off('devices:updated')
  Events.Off('rdp:local-status')
  Events.Off('rdp:status-updated')
})
</script>

<style scoped>
.devices-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: #f4f6f9;
}

.page-header {
  background: #ffffff;
  padding: 18px 28px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid #ebeef5;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.header-left h2 {
  margin: 0;
  font-size: 19px;
  font-weight: 600;
  color: #1f2937;
}

.devices-content {
  flex: 1;
  padding: 20px 28px 28px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.local-rdp-card {
  border-radius: 10px;
  border: 1px solid #e5e7eb;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.card-title-group {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.card-icon {
  font-size: 18px;
  color: #2563eb;
}

.card-title {
  font-size: 15px;
  font-weight: 600;
  color: #1f2937;
}

.current-device-meta {
  font-size: 12px;
  color: #6b7280;
}

.settings-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(260px, 1fr));
  gap: 16px;
}

@media (max-width: 1180px) {
  .settings-grid {
    grid-template-columns: 1fr;
  }
}

.setting-item {
  background: #f8fafc;
  border: 1px solid #eef2f7;
  border-radius: 8px;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.setting-label {
  font-size: 13px;
  font-weight: 600;
  color: #374151;
}

.setting-control {
  min-height: 32px;
  display: flex;
  align-items: center;
}

.setting-desc {
  font-size: 12px;
  line-height: 1.5;
  color: #6b7280;
}

.unattended-tip {
  margin-top: 14px;
  padding: 10px 14px;
  border-radius: 6px;
  background: #eff6ff;
  border: 1px solid #dbeafe;
  color: #1e40af;
  font-size: 12px;
  line-height: 1.6;
}

.section-block {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.section-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.section-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: #1f2937;
}

.section-subtitle {
  font-size: 12px;
  color: #6b7280;
}

.devices-table {
  background: #ffffff;
  border-radius: 10px;
  border: 1px solid #e5e7eb;
  overflow: hidden;
}

.device-primary {
  display: flex;
  align-items: center;
  gap: 8px;
}

.device-title {
  font-weight: 500;
  color: #111827;
}

.device-sub {
  font-size: 12px;
  color: #6b7280;
  margin-top: 2px;
}

.status-online {
  color: #16a34a;
  font-size: 13px;
  font-weight: 500;
}

.status-offline {
  color: #9ca3af;
  font-size: 13px;
}

.status-detail {
  color: #4b5563;
  font-weight: 400;
}

.rdp-status-cell {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.policy-controls {
  display: flex;
  align-items: center;
  gap: 10px;
}

.mode-select {
  width: 118px;
}

.action-buttons {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: nowrap;
}

.current-hint {
  font-size: 12px;
  color: #6b7280;
}

.expiry-text {
  font-size: 12px;
  color: #b45309;
}

.empty-state {
  padding: 36px 0;
  text-align: center;
  background: #ffffff;
  border-radius: 10px;
  border: 1px solid #e5e7eb;
}
</style>
