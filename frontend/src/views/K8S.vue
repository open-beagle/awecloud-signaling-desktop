<template>
  <div class="resource-page">
    <div class="page-header">
      <div>
        <h1>Kubernetes</h1>
        <p>当前账号可访问的 Kubernetes 集群</p>
      </div>
      <div class="manual-actions">
        <span v-if="domainsStore.lastFetchedAt" class="fetched-at">上次获取：{{ domainsStore.lastFetchedAt }}</span>
        <el-tooltip content="安装全部在线集群" placement="top">
          <button class="icon-btn" type="button" aria-label="安装全部在线集群" :disabled="!onlineK8sDomains.length" @click="openInstall">
            <el-icon><Download /></el-icon>
          </button>
        </el-tooltip>
        <button class="icon-btn" title="更新 Kubernetes 列表" :disabled="domainsStore.loading" @click="refreshDomains">
          <el-icon :class="{ 'is-loading': domainsStore.loading }"><Refresh /></el-icon>
        </button>
      </div>
    </div>

    <div class="toolbar">
      <el-input v-model="searchQuery" clearable placeholder="搜索集群、区域或地址" class="search-input" />
      <span class="count">{{ filteredDomains.length }} / {{ k8sDomains.length }} 个集群</span>
    </div>

    <el-table :data="filteredDomains" stripe height="100%" empty-text="暂无可访问集群">
      <el-table-column label="集群" min-width="210">
        <template #default="{ row }">
          <div class="primary">{{ clusterName(row) }}</div>
          <div class="secondary">{{ row.display_name || 'Kubernetes API' }}</div>
        </template>
      </el-table-column>
      <el-table-column label="区域" min-width="130">
        <template #default="{ row }">{{ row.region || '-' }}</template>
      </el-table-column>
      <el-table-column label="API 地址" min-width="300">
        <template #default="{ row }"><code>{{ kubernetesAPIURL(row.domain) }}</code></template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <span class="status"><i :class="row.status === 'offline' ? 'offline' : 'online'"></i>{{ row.status === 'offline' ? '离线' : '可用' }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="72" fixed="right" align="center">
        <template #default="{ row }">
          <div class="table-actions">
            <el-tooltip content="复制 kubeconfig" placement="top">
              <button class="icon-btn table-action" type="button" aria-label="复制 kubeconfig" @click="copyKubeconfig(row)">
                <el-icon><CopyDocument /></el-icon>
              </button>
            </el-tooltip>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="installDialogVisible" title="安装 kubeconfig" width="680px" :close-on-click-modal="false">
      <p class="dialog-description">将 {{ onlineK8sDomains.length }} 个在线集群写入所选环境的标准 kubeconfig。</p>

      <div v-loading="targetsLoading" class="install-targets">
        <label v-for="target in targets" :key="target.id" class="install-target" :class="{ selected: selectedTargets.includes(target.id), unavailable: !target.available }">
          <el-checkbox v-model="selectedTargets" :label="target.id" :disabled="!target.available">
            <span class="target-name">{{ target.name }}</span>
          </el-checkbox>
          <code>{{ target.path }}</code>
          <span class="target-hint">{{ target.hint }}</span>
        </label>
        <div v-if="!targetsLoading && !targets.length" class="target-empty">未检测到可安装环境</div>
      </div>

      <div class="install-policy">
        <strong>写入规则</strong>
        <span>配置不存在时自动创建；同名 context、cluster 和 user 直接覆盖；无关配置保留。</span>
      </div>

      <div v-if="installResults.length" class="install-results">
        <div v-for="result in installResults" :key="result.target_id" :class="result.success ? 'success' : 'failed'">
          <strong>{{ result.target_name }}</strong>
          <span>{{ result.success ? `${result.path} · ${result.context}` : result.error }}</span>
        </div>
      </div>

      <template #footer>
        <el-button @click="installDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="installing" :disabled="!selectedTargets.length" @click="installSelected">
          确认安装
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument, Download, Refresh } from '@element-plus/icons-vue'
import { GetKubeconfigTargets, InstallKubeconfig } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/internal/app/app'
import { useDomainsStore } from '../stores/domains'
import type { DomainItem } from '../stores/domains'
import { kubernetesAPIURL } from '../utils/kubernetes'

interface InstallTarget {
  id: string
  name: string
  kind: string
  path: string
  available: boolean
  hint: string
}

interface InstallTargetResult {
  target_id: string
  target_name: string
  path: string
  context: string
  success: boolean
  error: string
}

const domainsStore = useDomainsStore()
const k8sDomains = computed(() => domainsStore.k8sDomains)
const searchQuery = ref('')
const installDialogVisible = ref(false)
const targets = ref<InstallTarget[]>([])
const selectedTargets = ref<string[]>([])
const targetsLoading = ref(false)
const installing = ref(false)
const installResults = ref<InstallTargetResult[]>([])
const onlineK8sDomains = computed(() => k8sDomains.value.filter(domain => domain.status === 'online'))

const filteredDomains = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return k8sDomains.value
  return k8sDomains.value.filter(domain =>
    [domain.domain, domain.region, domain.display_name, clusterName(domain)]
      .some(value => value?.toLowerCase().includes(query))
  )
})

function clusterName(domain: DomainItem | null) {
  if (!domain) return ''
  const region = (domain.region || '').trim()
  if (region) return region.endsWith('.beagle') ? region : `${region}.beagle`
  return domain.domain.replace(/^kubernetes\./, '')
}

function kubeconfigFor(domain: DomainItem) {
  const name = clusterName(domain)
  return `apiVersion: v1
clusters:
- cluster:
    insecure-skip-tls-verify: true
    server: ${kubernetesAPIURL(domain.domain)}
  name: ${name}
contexts:
- context:
    cluster: ${name}
    user: who
  name: ${name}
current-context: ${name}
kind: Config
preferences: {}
users:
- name: who
  user:
    token: whoisyourdaddy`
}

async function copyKubeconfig(domain: DomainItem) {
  await navigator.clipboard.writeText(kubeconfigFor(domain))
  ElMessage.success('kubeconfig 已复制')
}

async function refreshDomains() {
  try {
    await domainsStore.refreshDomains()
  } catch (cause: any) {
    ElMessage.error(cause?.message || 'Kubernetes 列表更新失败')
  }
}

async function openInstall() {
  if (!onlineK8sDomains.value.length) return
  installResults.value = []
  installDialogVisible.value = true
  targetsLoading.value = true
  try {
    targets.value = ((await GetKubeconfigTargets()) || []).filter(Boolean) as InstallTarget[]
    selectedTargets.value = targets.value.filter(target => target.available && target.kind !== 'wsl').map(target => target.id)
  } catch (cause: any) {
    targets.value = []
    selectedTargets.value = []
    ElMessage.error(cause?.message || '安装环境检测失败')
  } finally {
    targetsLoading.value = false
  }
}

async function installSelected() {
  const currentDomain = onlineK8sDomains.value[0]
  if (!currentDomain || !selectedTargets.value.length) return
  installing.value = true
  installResults.value = []
  try {
    const result = await InstallKubeconfig({
      domain: currentDomain.domain,
      target_ids: selectedTargets.value
    })
    installResults.value = (result?.targets || []).filter(Boolean) as InstallTargetResult[]
    const successful = installResults.value.filter(item => item.success).length
    if (successful === installResults.value.length) ElMessage.success('kubeconfig 安装完成')
    else if (successful) ElMessage.warning('部分环境安装失败，请查看结果')
    else ElMessage.error('kubeconfig 安装失败')
  } catch (cause: any) {
    ElMessage.error(cause?.message || 'kubeconfig 安装失败')
  } finally {
    installing.value = false
  }
}
</script>

<style scoped src="../styles/resource-list.css"></style>
<style scoped>
.table-actions { display: flex; justify-content: flex-end; gap: 6px; }
.install-targets { min-height: 92px; display: grid; gap: 8px; }
.install-target { min-height: 64px; display: grid; grid-template-columns: 170px minmax(0, 1fr) auto; align-items: center; gap: 12px; padding: 9px 11px; border: 1px solid #dfe5ed; border-radius: 7px; cursor: pointer; }
.install-target.selected { background: #f5f9ff; border-color: #bed2f1; }
.install-target.unavailable { cursor: not-allowed; opacity: .68; }
.target-name { font-weight: 600; }
.target-hint { color: #7a8799; font-size: 11px; white-space: nowrap; }
.target-empty { padding: 30px; color: #909399; text-align: center; }
.install-policy { display: flex; flex-direction: column; gap: 5px; margin-top: 12px; padding: 10px 12px; color: #606266; background: #f5f7fa; border-radius: 7px; font-size: 12px; }
.install-results { display: grid; gap: 7px; margin-top: 12px; }
.install-results > div { display: grid; grid-template-columns: 140px minmax(0, 1fr); gap: 10px; padding: 9px 11px; border-radius: 6px; font-size: 12px; }
.install-results .success { color: #2e6f4b; background: #edf8f2; }
.install-results .failed { color: #b33b3b; background: #fff2f0; }
.install-results span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
