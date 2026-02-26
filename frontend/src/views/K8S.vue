<template>
  <div class="k8s-page">
    <div v-if="k8sDomains.length === 0" class="empty">
      暂无可用的 K8S 集群
    </div>
    
    <div v-else class="k8s-grid">
      <K8SCard
        v-for="domain in k8sDomains"
        :key="domain.domain"
        :domain="domain"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import K8SCard from '../components/K8SCard.vue'
import { useDomainsStore } from '../stores/domains'

const domainsStore = useDomainsStore()

// 获取 K8SAPI 类型的域名列表
const k8sDomains = computed(() => domainsStore.k8sDomains)
</script>

<style scoped>
.k8s-page {
  padding: 20px;
}

.empty {
  text-align: center;
  padding: 40px;
  color: #999;
  font-size: 14px;
}

.k8s-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}
</style>
