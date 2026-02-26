<template>
  <div class="services-page">
    <div v-if="servicesDomains.length === 0" class="empty">
      暂无服务
    </div>
    
    <div v-else class="services-grid">
      <ServiceDomainCard
        v-for="domain in servicesDomains"
        :key="domain.domain"
        :domain="domain"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useDomainsStore } from '../stores/domains'
import ServiceDomainCard from '../components/ServiceDomainCard.vue'

const domainsStore = useDomainsStore()

const servicesDomains = computed(() => domainsStore.servicesDomains)
</script>

<style scoped>
.services-page {
  padding: 20px;
}

.empty {
  text-align: center;
  padding: 40px;
  color: #999;
  font-size: 14px;
}

.services-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}
</style>
