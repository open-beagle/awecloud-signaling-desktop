<template>
  <div class="login-container">
    <div class="login-box">
      <div class="logo-container">
        <img src="../assets/logo.png" alt="AWECloud Logo" class="logo" />
      </div>
      <h1 class="title">Signaling Desktop</h1>
      <p class="version">{{ appVersion }}</p>
      <p class="subtitle">连接到您的远程服务</p>

      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-width="100px"
        class="login-form"
      >
        <el-form-item label="服务器地址" prop="serverAddress">
          <el-input
            v-model="form.serverAddress"
            placeholder="例如: localhost:8081"
            :disabled="loading"
          />
        </el-form-item>

        <el-form-item label="Client ID" prop="clientId">
          <el-input
            v-model="form.clientId"
            placeholder="用户名或邮箱"
            :disabled="loading"
          />
        </el-form-item>

        <el-form-item label="Client Secret" prop="clientSecret">
          <el-input
            v-model="form.clientSecret"
            type="password"
            placeholder="请输入密钥"
            :disabled="loading"
            show-password
          />
        </el-form-item>

        <el-form-item>
          <el-checkbox v-model="form.rememberMe">
            记住登录（7天有效）
          </el-checkbox>
        </el-form-item>

        <el-form-item>
          <el-button
            type="primary"
            :loading="loading"
            @click="handleLogin"
            style="width: 100%"
          >
            {{ loading ? '登录中...' : '登录' }}
          </el-button>
        </el-form-item>
      </el-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { useAuthStore } from '../stores/auth'
import { Login, GetVersion, CheckSavedCredentials } from '../../wailsjs/go/main/App'

const router = useRouter()
const authStore = useAuthStore()
const appVersion = ref('dev')

const formRef = ref<FormInstance>()
const loading = ref(false)

const form = reactive({
  serverAddress: authStore.serverAddress || 'localhost:9090',
  clientId: '',
  clientSecret: '',
  rememberMe: true  // 默认勾选
})

const rules: FormRules = {
  serverAddress: [
    { required: true, message: '请输入服务器地址', trigger: 'blur' }
  ],
  clientId: [
    { required: true, message: '请输入 Client ID', trigger: 'blur' }
  ],
  clientSecret: [
    { required: true, message: '请输入 Client Secret', trigger: 'blur' }
  ]
}

const handleLogin = async () => {
  if (!formRef.value) return

  await formRef.value.validate(async (valid) => {
    if (!valid) return

    loading.value = true
    try {
      // 调用 Go 后端的 Login 方法
      await Login(
        form.serverAddress,
        form.clientId,
        form.clientSecret,
        form.rememberMe
      )

      // 登录成功
      authStore.setAuthenticated(true)
      authStore.setServerAddress(form.serverAddress)
      authStore.setClientId(form.clientId)

      ElMessage.success('登录成功')
      router.push('/services')
    } catch (error: any) {
      ElMessage.error(error.message || '登录失败')
    } finally {
      loading.value = false
    }
  })
}

// 获取版本信息和检查保存的凭据
onMounted(async () => {
  try {
    const versionInfo = await GetVersion()
    appVersion.value = versionInfo.version
  } catch (error) {
    console.error('Failed to get version:', error)
  }

  // 检查是否有保存的凭据
  try {
    const savedCreds = await CheckSavedCredentials()
    if (savedCreds) {
      form.serverAddress = savedCreds.server_address
      form.clientId = savedCreds.client_id
      form.clientSecret = savedCreds.client_secret
      form.rememberMe = savedCreds.remember_me

      // 自动登录
      ElMessage.info('使用保存的凭据自动登录...')
      handleLogin()
    }
  } catch (error) {
    console.error('Failed to check saved credentials:', error)
  }
})
</script>

<style scoped>
.login-container {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100vh;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.login-box {
  background: white;
  padding: 40px;
  border-radius: 10px;
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.1);
  width: 400px;
}

.logo-container {
  text-align: center;
  margin-bottom: 20px;
}

.logo {
  width: 80px;
  height: 80px;
  object-fit: contain;
}

.title {
  text-align: center;
  margin: 0 0 5px 0;
  font-size: 28px;
  color: #333;
}

.version {
  text-align: center;
  margin: 0 0 10px 0;
  color: #999;
  font-size: 12px;
  font-weight: normal;
}

.subtitle {
  text-align: center;
  margin: 0 0 30px 0;
  color: #666;
  font-size: 14px;
}

.login-form {
  margin-top: 20px;
}
</style>
