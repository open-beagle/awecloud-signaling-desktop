<template>
  <div class="service-card" :class="{ offline: domain.status === 'offline' }">
    <div class="card-header">
      <span class="status-indicator" :class="domain.status"></span>
      <span class="domain-name">{{ domain.domain }}</span>
    </div>
    <div class="card-body">
      <div class="ports-list">
        <span v-for="port in domain.service_ports" :key="port" class="port-tag">
          {{ port }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { DomainItem } from '../stores/domains'

defineProps<{
  domain: DomainItem
}>()
</script>

<style scoped>
.service-card {
  background: #fff;
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  padding: 16px;
  cursor: pointer;
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
}

.card-body {
  padding-top: 8px;
  border-top: 1px solid #f0f0f0;
}

.ports-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.port-tag {
  display: inline-block;
  padding: 2px 8px;
  background-color: #f0f0f0;
  border-radius: 4px;
  font-size: 12px;
  color: #666;
}
</style>
