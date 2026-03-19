<template>
  <div class="hosts-page">
    <div v-if="hostsDomains.length === 0" class="empty">
      暂无可用主机
    </div>

    <template v-else>
      <div class="toolbar">
        <input
          v-model="searchQuery"
          class="search-input"
          placeholder="搜索主机..."
          type="text"
        />
        <span class="count">{{ filteredDomains.length }} / {{ hostsDomains.length }} 个主机</span>
      </div>

      <div v-if="filteredDomains.length === 0" class="empty">
        未找到匹配的主机
      </div>

      <div v-else class="hosts-grid">
        <HostCard
          v-for="domain in filteredDomains"
          :key="domain.domain"
          :domain="domain"
        />
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useDomainsStore } from '../stores/domains'
import HostCard from '../components/HostCard.vue'

const domainsStore = useDomainsStore()
const hostsDomains = computed(() => domainsStore.hostsDomains)
const searchQuery = ref('')

const filteredDomains = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return hostsDomains.value
  return hostsDomains.value.filter(d =>
    d.domain.toLowerCase().includes(q) ||
    (d.ssh_users && d.ssh_users.some(u => u.toLowerCase().includes(q)))
  )
})
</script>

<style scoped>
.hosts-page {
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
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.search-input {
  flex: 1;
  max-width: 260px;
  padding: 5px 10px;
  border: 1px solid #d9d9d9;
  border-radius: 4px;
  font-size: 13px;
  outline: none;
  transition: border-color 0.2s;
}

.search-input:focus {
  border-color: #1890ff;
}

.count {
  font-size: 13px;
  color: #999;
}

.hosts-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
  padding-bottom: 20px;
}
</style>