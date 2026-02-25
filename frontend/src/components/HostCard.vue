<template>
  <div class="host-card" :class="{ offline: domain.status === 'offline' }">
    <div class="card-header">
      <span class="status-indicator" :class="domain.status"></span>
      <span class="domain-name">{{ domain.domain }}</span>
    </div>
    <div class="card-body">
      <div class="users-list">
        <div v-for="user in domain.ssh_users" :key="user" class="user-item">
          <span class="user-name">{{ user }}</span>
          <button class="copy-btn" @click.stop="copySSHCommand(user)">
            {{ copiedUser === user ? '已复制' : '复制' }}
          </button>
        </div>
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

function copySSHCommand(user: string) {
  const command = `ssh ${user}@${props.domain.domain}`
  navigator.clipboard.writeText(command)
  copiedUser.value = user
  setTimeout(() => {
    copiedUser.value = null
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
  justify-content: space-between;
  padding: 6px 0;
}

.user-name {
  font-size: 13px;
  color: #666;
  font-family: monospace;
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
