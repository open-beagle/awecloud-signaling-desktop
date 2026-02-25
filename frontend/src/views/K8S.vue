<template>
  <Layout>
    <div class="k8s-page">
      <div class="page-header">
        <h2>我的K8S</h2>
      </div>

      <div class="k8s-content">
        <el-empty 
          v-if="k8sDomains.length === 0" 
          description="暂无可用的 K8S 集群" 
        />
        
        <div v-else class="k8s-grid">
          <K8SCard
            v-for="domain in k8sDomains"
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
import K8SCard from '../components/K8SCard.vue'
import { useDomainsStore } from '../stores/domains'

const domainsStore = useDomainsStore()

// 获取 K8SAPI 类型的域名列表
const k8sDomains = computed(() => domainsStore.k8sDomains)
</script>

<style scoped>
.k8s-page {
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

.k8s-content {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
}

.k8s-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
  gap: 20px;
}
</style>
