<template>
  <div class="services-page">
    <div class="page-header">
      <h2>我的服务</h2>
    </div>
    
    <div v-if="loading" class="loading">
      加载中...
    </div>
    
    <div v-else-if="servicesDomains.length === 0" class="empty">
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
import { computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { useDomainsStore } from '../stores/domains'
import ServiceDomainCard from '../components/ServiceDomainCard.vue'
import { GetDomainList } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/app'

const domainsStore = useDomainsStore()

const loading = computed(() => domainsStore.loading)
const servicesDomains = computed(() => domainsStore.servicesDomains)

const loadDomains = async () => {
  domainsStore.setLoading(true)
  try {
    const domains = await GetDomainList()
    domainsStore.setDomains(domains || [])
  } catch (error: any) {
    ElMessage.error(error.message || '获取域名列表失败')
  } finally {
    domainsStore.setLoading(false)
  }
}

onMounted(() => {
  loadDomains()
})
</script>

<style scoped>
.services-page {
  padding: 20px;
}

.page-header {
  margin-bottom: 20px;
}

.page-header h2 {
  font-size: 20px;
  font-weight: 600;
  color: #333;
  margin: 0;
}

.loading,
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
