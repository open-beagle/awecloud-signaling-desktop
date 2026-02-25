<template>
  <Layout>
    <div class="k8s-detail-page">
      <div class="page-header">
        <h2>{{ domain }}</h2>
      </div>

      <div class="detail-content">
        <el-card v-if="!domainData" shadow="never">
          <el-empty description="集群不存在或无权访问" />
        </el-card>

        <el-card v-else shadow="never" class="kubeconfig-card">
          <template #header>
            <div class="card-header">
              <span>Kubeconfig</span>
              <el-button 
                type="primary" 
                @click="copyKubeconfig"
                :disabled="!kubeconfigText"
              >
                {{ copyButtonText }}
              </el-button>
            </div>
          </template>

          <div class="kubeconfig-container">
            <pre class="kubeconfig-text">{{ kubeconfigText }}</pre>
          </div>
        </el-card>
      </div>
    </div>
  </Layout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import Layout from '../components/Layout.vue'
import { useDomainsStore } from '../stores/domains'

const route = useRoute()
const domainsStore = useDomainsStore()

const domain = computed(() => route.params.domain as string)
const copyButtonText = ref('复制 kubeconfig')

// 查找当前域名的数据
const domainData = computed(() => {
  return domainsStore.domains.find(d => d.domain === domain.value)
})

// 从域名中提取 region（格式：kubernetes.{region}.beagle）
const region = computed(() => {
  if (!domain.value) return ''
  const parts = domain.value.split('.')
  if (parts.length >= 3) {
    return parts[1] // 取第二部分作为 region
  }
  return 'default'
})

// 生成 kubeconfig 文本
const kubeconfigText = computed(() => {
  if (!domainData.value) return ''

  const reg = region.value
  const dom = domain.value

  return `apiVersion: v1
kind: Config
clusters:
- name: ${reg}
  cluster:
    server: https://${dom}:6443
    insecure-skip-tls-verify: true
contexts:
- name: ${reg}
  context:
    cluster: ${reg}
    user: anonymous
current-context: ${reg}
users:
- name: anonymous
  user:
    token: anonymous`
})

// 复制 kubeconfig 到剪贴板
const copyKubeconfig = async () => {
  if (!kubeconfigText.value) return

  try {
    await navigator.clipboard.writeText(kubeconfigText.value)
    copyButtonText.value = '已复制'
    ElMessage.success('已复制到剪贴板')
    
    setTimeout(() => {
      copyButtonText.value = '复制 kubeconfig'
    }, 1500)
  } catch (error) {
    ElMessage.error('复制失败')
  }
}
</script>

<style scoped>
.k8s-detail-page {
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

.detail-content {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
}

.kubeconfig-card {
  max-width: 900px;
  margin: 0 auto;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-header span {
  font-size: 16px;
  font-weight: 500;
}

.kubeconfig-container {
  background: #f8f9fa;
  border-radius: 4px;
  padding: 16px;
  overflow-x: auto;
}

.kubeconfig-text {
  margin: 0;
  font-family: 'Courier New', Consolas, monospace;
  font-size: 13px;
  line-height: 1.6;
  color: #333;
  white-space: pre;
}
</style>
