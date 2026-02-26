<template>
  <div class="hosts-page">
    <div v-if="hostsDomains.length === 0" class="empty">
      暂无可用主机
    </div>
    
    <div v-else class="hosts-grid">
      <HostCard
        v-for="domain in hostsDomains"
        :key="domain.domain"
        :domain="domain"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useDomainsStore } from '../stores/domains'
import HostCard from '../components/HostCard.vue'

const domainsStore = useDomainsStore()

const hostsDomains = computed(() => domainsStore.hostsDomains)
</script>

<style scoped>
.hosts-page {
  padding: 20px;
}

.empty {
  text-align: center;
  padding: 40px;
  color: #999;
  font-size: 14px;
}

.hosts-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}
</style>
