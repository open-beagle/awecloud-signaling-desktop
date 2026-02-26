<template>
  <div class="host-card" :class="{ offline: domain.status === 'offline' }">
    <div class="card-header">
      <span class="status-indicator" :class="domain.status"></span>
      <span class="domain-name">{{ domain.domain }}</span>
      <button class="copy-btn domain-copy" @click.stop="copyDomain" :title="'复制域名'">
        {{ domainCopied ? '✓' : '⎘' }}
      </button>
    </div>
    <div class="card-body">
      <!-- SSH 用户列表 -->
      <div v-if="domain.ssh_users && domain.ssh_users.length > 0" class="users-list">
        <div v-for="user in domain.ssh_users" :key="user" class="user-item">
          <span class="user-label">SSH</span>
          <span class="user-command">{{ user }}@{{ domain.domain }}</span>
          <button class="copy-btn" @click.stop="copySSHCommand(user)">
            {{ copiedUser === user ? '已复制' : '复制' }}
          </button>
        </div>
      </div>
      <!-- 无用户时显示快速复制域名 -->
      <div v-else class="no-users">
        <span class="hint-text">暂无 SSH 用户</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import type { DomainItem } from '../stores/domains'

const props = defineProps<{
  domain: DomainItem
}>()

const copiedUser = ref<string | null>(null)
const domainCopied = ref(false)

function copySSHCommand(user: string) {
  const command = `ssh ${user}@${props.domain.domain}`
  navigator.clipboard.writeText(command)
  copiedUser.value = user
  setTimeout(() => {
    copiedUser.value = null
  }, 1500)
}

function copyDomain() {
  navigator.clipboard.writeText(props.domain.domain)
  domainCopied.value = true
  setTimeout(() => {
    domainCopied.value = false
  }, 1500)
}
</script>

<style scoped>
.host-card {
  background: #fff;
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  padding: 16px;
  transition: all 0.3s;
}

.host-card:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
}

.host-card.offline {
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
  flex: 1;
}

.domain-copy {
  padding: 2px 6px;
  font-size: 14px;
  background: transparent;
  color: #999;
  border: 1px solid #e0e0e0;
  border-radius: 4px;
  cursor: pointer;
  flex-shrink: 0;
}

.domain-copy:hover {
  color: #1890ff;
  border-color: #1890ff;
}

.card-body {
  padding-top: 8px;
  border-top: 1px solid #f0f0f0;
}

.users-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.user-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
}

.user-label {
  font-size: 11px;
  color: #fff;
  background: #52c41a;
  padding: 1px 6px;
  border-radius: 3px;
  flex-shrink: 0;
}

.user-command {
  font-size: 13px;
  color: #666;
  font-family: 'Consolas', 'Monaco', monospace;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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
  flex-shrink: 0;
}

.copy-btn:hover {
  background-color: #40a9ff;
}

.copy-btn:active {
  background-color: #096dd9;
}

.no-users {
  padding: 8px 0;
}

.hint-text {
  font-size: 12px;
  color: #999;
}
</style>
