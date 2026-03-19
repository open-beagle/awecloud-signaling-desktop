<template>
  <div class="services-page">
    <div v-if="servicesDomains.length === 0" class="empty">
      暂无服务
    </div>

    <template v-else>
      <div class="toolbar">
        <input
          v-model="searchQuery"
          class="search-input"
          placeholder="搜索服务..."
          type="text"
        />
        <span class="count">{{ filteredDomains.length }} / {{ servicesDomains.length }} 个服务</span>
      </div>

      <div v-if="filteredDomains.length === 0" class="empty">
        未找到匹配的服务
      </div>

      <div v-else class="services-grid">
        <ServiceDomainCard
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
import ServiceDomainCard from '../components/ServiceDomainCard.vue'

const domainsStore = useDomainsStore()
const servicesDomains = computed(() => domainsStore.servicesDomains)
const searchQuery = ref('')

const filteredDomains = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return servicesDomains.value
  return servicesDomains.value.filter(d =>
    d.domain.toLowerCase().includes(q) ||
    (d.namespace && d.namespace.toLowerCase().includes(q)) ||
    (d.service_name && d.service_name.toLowerCase().includes(q))
  )
})
</script>

<style scoped>
.services-page {
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

.services-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
  padding-bottom: 20px;
}
</style>