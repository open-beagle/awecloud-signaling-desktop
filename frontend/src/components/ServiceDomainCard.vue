<template>
  <div class="service-card" :class="{ offline: domain.status === 'offline' }">
    <div class="card-header">
      <span class="status-indicator" :class="domain.status"></span>
      <span class="domain-name">{{ domain.domain }}</span>
      <button class="copy-btn domain-copy" @click.stop="copyDomain" :title="'复制域名'">
        {{ domainCopied ? '✓' : '⎘' }}
      </button>
    </div>
    <div class="card-body">
      <!-- 服务信息 -->
      <div v-if="domain.namespace || domain.service_name" class="service-info">
        <span v-if="domain.namespace" class="info-tag namespace">{{ domain.namespace }}</span>
        <span v-if="domain.service_name" class="info-tag svc-name">{{ domain.service_name }}</span>
      </div>
      <!-- 端口列表 -->
      <div v-if="domain.service_ports && domain.service_ports.length > 0" class="ports-list">
        <div v-for="port in domain.service_ports" :key="port" class="port-item">
          <span class="port-tag">{{ port }}</span>
          <button class="copy-btn small" @click.stop="copyAddress(port)">
            {{ copiedPort === port ? '已复制' : '复制' }}
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

const copiedPort = ref<number | null>(null)
const domainCopied = ref(false)

function copyAddress(port: number) {
  const address = `${props.domain.domain}:${port}`
  navigator.clipboard.writeText(address)
  copiedPort.value = port
  setTimeout(() => {
    copiedPort.value = null
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
.service-card {
  background: #fff;
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  padding: 16px;
  transition: all 0.3s;
}

.service-card:hover {
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
  transform: translateY(-2px);
}

.service-card.offline {
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

.service-info {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 8px;
}

.info-tag {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 3px;
  font-size: 11px;
}

.info-tag.namespace {
  background: #e6f7ff;
  color: #1890ff;
}

.info-tag.svc-name {
  background: #f6ffed;
  color: #52c41a;
}

.ports-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.port-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 0;
}

.port-tag {
  display: inline-block;
  padding: 2px 8px;
  background-color: #f0f0f0;
  border-radius: 4px;
  font-size: 12px;
  color: #666;
  font-family: 'Consolas', 'Monaco', monospace;
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

.copy-btn.small {
  padding: 2px 10px;
  font-size: 11px;
}
</style>
