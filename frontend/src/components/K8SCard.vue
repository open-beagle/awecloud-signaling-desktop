<template>
  <div class="k8s-card" :class="{ offline: domain.status === 'offline' }" @click="handleClick">
    <div class="card-header">
      <span class="status-indicator" :class="domain.status"></span>
      <span class="domain-name">{{ domain.domain }}</span>
    </div>
    <div class="card-footer">
      <button class="copy-btn" @click.stop="copyKubeconfig">
        {{ copied ? '已复制' : '复制' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import type { DomainItem } from '../stores/domains'

const props = defineProps<{
  domain: DomainItem
}>()

const router = useRouter()
const copied = ref(false)

function handleClick() {
  // 点击卡片进入详情页
  router.push(`/k8s/${encodeURIComponent(props.domain.domain)}`)
}

function copyKubeconfig() {
  // 生成 kubeconfig
  const kubeconfig = generateKubeconfig(props.domain)
  navigator.clipboard.writeText(kubeconfig)
  copied.value = true
  setTimeout(() => {
    copied.value = false
  }, 1500)
}

function generateKubeconfig(domain: DomainItem): string {
  const region = domain.region
  return `apiVersion: v1
clusters:
- name: ${region}
  cluster:
    insecure-skip-tls-verify: true
    server: https://${domain.domain}:6443
contexts:
- context:
    cluster: ${region}
    user: anonymous
  name: ${region}
current-context: ${region}
kind: Config
preferences: {}
users:
- name: anonymous
  user:
    token: anonymous`
}
</script>

<style scoped>
.k8s-card {
  background: #fff;
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  padding: 16px;
  cursor: pointer;
  transition: all 0.3s;
}

.k8s-card:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  transform: translateY(-2px);
}

.k8s-card.offline {
  opacity: 0.6;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.status-indicator {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.status-indicator.online {
  background-color: #52c41a;
}

.status-indicator.offline {
  background-color: #d9d9d9;
  border: 1px solid #999;
}

.domain-name {
  font-size: 14px;
  font-weight: 500;
  color: #333;
  word-break: break-all;
}

.card-footer {
  padding-top: 8px;
  border-top: 1px solid #f0f0f0;
  display: flex;
  justify-content: flex-end;
}

.copy-btn {
  padding: 4px 12px;
  background-color: #1890ff;
  color: #fff;
  border: none;
  border-radius: 4px;
  font-size: 12px;
  cursor: pointer;
  transition: background-color 0.3s;
}

.copy-btn:hover {
  background-color: #40a9ff;
}

.copy-btn:active {
  background-color: #096dd9;
}
</style>
