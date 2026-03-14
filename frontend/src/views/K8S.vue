<template>
  <div class="k8s-page">
    <div v-if="k8sDomains.length === 0" class="empty">
      暂无可用的 K8S 集群
    </div>

    <template v-else>
      <div class="toolbar">
        <span class="cluster-count">共 {{ k8sDomains.length }} 个集群</span>
        <button class="merge-btn" @click="copyAllKubeconfig">
          {{ allCopied ? '✓ 已复制' : '复制全部 kubeconfig' }}
        </button>
      </div>

      <div class="k8s-grid">
        <K8SCard
          v-for="domain in k8sDomains"
          :key="domain.domain"
          :domain="domain"
        />
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import K8SCard from '../components/K8SCard.vue'
import { useDomainsStore } from '../stores/domains'
import type { DomainItem } from '../stores/domains'

const domainsStore = useDomainsStore()
const k8sDomains = computed(() => domainsStore.k8sDomains)
const allCopied = ref(false)

function generateMergedKubeconfig(domains: DomainItem[]): string {
  const clusters = domains.map(d => `- name: ${d.region}
  cluster:
    insecure-skip-tls-verify: true
    server: https://${d.domain}:6443`).join('\n')

  const contexts = domains.map(d => `- context:
    cluster: ${d.region}
    user: anonymous
  name: ${d.region}`).join('\n')

  // 只需要一个 anonymous user
  const users = `- name: anonymous
  user:
    token: anonymous`

  return `apiVersion: v1
kind: Config
preferences: {}
current-context: ${domains[0].region}
clusters:
${clusters}
contexts:
${contexts}
users:
${users}`
}

function copyAllKubeconfig() {
  const merged = generateMergedKubeconfig(k8sDomains.value)
  navigator.clipboard.writeText(merged)
  allCopied.value = true
  setTimeout(() => { allCopied.value = false }, 1500)
}
</script>

<style scoped>
.k8s-page {
  padding: 20px;
  flex: 1;
  overflow-y: auto;
  min-height: 0;
}

.empty {
  text-align: center;
  padding: 40px;
  color: #999;
  font-size: 14px;
}

.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.cluster-count {
  font-size: 13px;
  color: #999;
}

.merge-btn {
  padding: 6px 16px;
  background-color: #1890ff;
  color: #fff;
  border: none;
  border-radius: 4px;
  font-size: 13px;
  cursor: pointer;
  transition: background-color 0.3s;
}

.merge-btn:hover {
  background-color: #40a9ff;
}

.merge-btn:active {
  background-color: #096dd9;
}

.k8s-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
  padding-bottom: 20px;
}
</style>
