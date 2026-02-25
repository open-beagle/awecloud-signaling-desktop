<template>
  <Layout>
    <div class="hosts-page">
      <div class="page-header">
        <h2>我的主机</h2>
      </div>

      <div class="hosts-content">
        <el-empty 
          v-if="hostsDomains.length === 0" 
          description="暂无可用主机" 
        />
        
        <div v-else class="hosts-grid">
          <HostCard
            v-for="domain in hostsDomains"
            :key="domain.domain"
            :domain="domain"
          />
        </div>
      </div>
    </div>
  </Layout>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Layout from '../components/Layout.vue'
import HostCard from '../components/HostCard.vue'
import { useDomainsStore } from '../stores/domains'

const domainsStore = useDomainsStore()

// 获取 SSH 类型的域名列表
const hostsDomains = computed(() => domainsStore.hostsDomains)
</script>

<style scoped>
.hosts-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: #f5f5f5;
}

.page-header {
  background: white;
  padding: 20px 30px;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
}

.page-header h2 {
  margin: 0;
  font-size: 20px;
  font-weight: 500;
  color: #333;
}

.hosts-content {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
}

.hosts-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
  gap: 20px;
}
</style>
