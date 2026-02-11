<template>
  <Layout>
    <div class="resources-page">
      <!-- 页面头部 -->
      <div class="page-header">
        <div class="header-left">
          <h2>资源发现</h2>
        </div>
        <div class="header-right">
          <el-tooltip content="刷新" placement="bottom">
            <el-button :icon="Refresh" @click="handleRefresh" :loading="loading" circle />
          </el-tooltip>
        </div>
      </div>

      <!-- Tab 切换 -->
      <div class="resources-content">
        <el-tabs v-model="activeTab" class="resource-tabs">
          <!-- SSH Tab -->
          <el-tab-pane :label="`SSH (${sshResources.length})`" name="ssh">
            <el-empty v-if="sshResources.length === 0" description="暂无 SSH 资源" />
            <div v-else class="resource-list">
              <div v-for="r in sshResources" :key="r.domain" class="resource-card">
                <div class="resource-info">
                  <div class="resource-name">{{ r.agent_name }}</div>
                  <div class="resource-domain">{{ r.domain || '域名未注册' }}</div>
                  <div class="resource-detail" v-if="r.ssh_users?.length">
                    用户: {{ r.ssh_users.join(', ') }}
                  </div>
                </div>
                <div class="resource-actions">
                  <el-button
                    v-if="r.domain"
                    size="small"
                    type="primary"
                    @click="copyCommand(sshCommand(r))"
                  >复制命令</el-button>
                </div>
              </div>
            </div>
          </el-tab-pane>

          <!-- K8S API Tab -->
          <el-tab-pane :label="`K8S API (${k8sApiResources.length})`" name="k8sapi">
            <el-empty v-if="k8sApiResources.length === 0" description="暂无 K8S API 资源" />
            <div v-else class="resource-list">
              <div v-for="r in k8sApiResources" :key="r.domain" class="resource-card">
                <div class="resource-info">
                  <div class="resource-name">{{ r.agent_name }}</div>
                  <div class="resource-domain">{{ r.domain || '域名未注册' }}</div>
                  <div class="resource-detail" v-if="r.namespaces?.length">
                    命名空间: {{ r.namespaces.join(', ') }}
                  </div>
                </div>
                <div class="resource-actions">
                  <el-button
                    v-if="r.domain"
                    size="small"
                    type="primary"
                    @click="copyCommand(k8sApiCommand(r))"
                  >复制命令</el-button>
                </div>
              </div>
            </div>
          </el-tab-pane>

          <!-- K8S Service Tab -->
          <el-tab-pane :label="`K8S Service (${k8sSvcResources.length})`" name="k8ssvc">
            <el-empty v-if="k8sSvcResources.length === 0" description="暂无 K8S Service 资源" />
            <div v-else class="resource-list">
              <div v-for="r in k8sSvcResources" :key="r.domain + ':' + r.port" class="resource-card">
                <div class="resource-info">
                  <div class="resource-name">{{ r.service_name }}</div>
                  <div class="resource-domain">{{ r.domain || '域名未注册' }}</div>
                  <div class="resource-detail">
                    {{ r.agent_name }} / {{ r.namespace }} · 端口 {{ r.port }}
                  </div>
                </div>
                <div class="resource-actions">
                  <el-button
                    v-if="r.domain"
                    size="small"
                    type="primary"
                    @click="copyCommand(k8sSvcCommand(r))"
                  >复制命令</el-button>
                </div>
              </div>
            </div>
          </el-tab-pane>
        </el-tabs>
      </div>
    </div>
  </Layout>
</template>

<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import Layout from '../components/Layout.vue'
import { GetResources } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/app'

interface Resource {
  type: string
  agent_id: number
  agent_name: string
  domain: string
  ssh_users?: string[]
  k8s_groups?: string[]
  namespaces?: string[]
  namespace?: string
  service_name?: string
  port?: number
}

const loading = ref(false)
const activeTab = ref('ssh')
const resources = ref<Resource[]>([])

const sshResources = computed(() => resources.value.filter(r => r.type === 'ssh'))
const k8sApiResources = computed(() => resources.value.filter(r => r.type === 'k8sapi'))
const k8sSvcResources = computed(() => resources.value.filter(r => r.type === 'k8ssvc'))

// 生成连接命令
const sshCommand = (r: Resource) => {
  const user = r.ssh_users?.[0] || 'root'
  return `ssh ${user}@${r.domain}`
}

const k8sApiCommand = (r: Resource) => {
  return `kubectl --server=https://${r.domain}:6443 get pods`
}

const k8sSvcCommand = (r: Resource) => {
  return `# ${r.service_name} @ ${r.domain}:${r.port}`
}

// 复制命令到剪贴板
const copyCommand = async (cmd: string) => {
  try {
    await navigator.clipboard.writeText(cmd)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败')
  }
}

const loadResources = async () => {
  loading.value = true
  try {
    const result = await GetResources()
    resources.value = (result || []).filter((r): r is NonNullable<typeof r> => r !== null) as Resource[]
  } catch (error: any) {
    ElMessage.error(error.message || '获取资源列表失败')
  } finally {
    loading.value = false
  }
}

const handleRefresh = async () => {
  await loadResources()
  ElMessage.success('刷新成功')
}

onMounted(() => {
  loadResources()
})
</script>

<style scoped>
.resources-page {
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

.header-left h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 500;
  color: #333;
}

.resources-content {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
}

.resource-tabs :deep(.el-tabs__header) {
  background: white;
  padding: 0 16px;
  border-radius: 8px;
  margin-bottom: 16px;
}

.resource-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.resource-card {
  background: white;
  border-radius: 8px;
  padding: 16px 20px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.06);
  transition: box-shadow 0.2s;
}

.resource-card:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.12);
}

.resource-name {
  font-size: 15px;
  font-weight: 500;
  color: #333;
}

.resource-domain {
  font-size: 13px;
  color: #409eff;
  font-family: monospace;
  margin-top: 4px;
}

.resource-detail {
  font-size: 12px;
  color: #999;
  margin-top: 4px;
}
</style>
