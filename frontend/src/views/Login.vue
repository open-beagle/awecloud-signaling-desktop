<template>
  <div class="login-container">
    <UpgradeDialog ref="upgradeDialogRef" />
    <div class="login-box">
      <div class="logo-container">
        <div class="logo-wrapper" :class="{ 'is-loading': isAutoLogging }">
          <img src="../assets/logo.png" alt="AWECloud Logo" class="logo" />
        </div>
      </div>
      <h1 class="title">Signaling Desktop</h1>
      <p class="version">{{ appVersion }}</p>
      <p class="subtitle">{{ isAutoLogging ? '正在自动登录' + loadingDots : '连接到您的远程服务' }}</p>

      <!-- 离线模式提示 -->
      <el-alert
        v-if="loginMode === 'offline'"
        type="warning"
        :closable="false"
        show-icon
        class="offline-alert"
      >
        <template #title>
          服务器离线
        </template>
        <p>无法连接到服务器，您可以查看缓存的服务但无法连接新服务。</p>
        <p class="offline-info">
          <strong>服务器：</strong>{{ form.server }}<br />
          <strong>用户：</strong>{{ form.client }}
        </p>
      </el-alert>

      <!-- 登录提示 -->
      <el-alert
        v-if="loginMode === 'full' && loginHint && !isAutoLogging"
        type="info"
        :closable="false"
        class="login-hint"
      >
        {{ loginHint }}
      </el-alert>

      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-width="100px"
        class="login-form"
      >
        <!-- 模式1：离线模式 - 只读显示 -->
        <template v-if="loginMode === 'offline'">
          <el-form-item label=" " class="button-form-item">
            <el-button
              type="primary"
              :loading="reconnecting"
              @click="handleReconnect"
              class="full-width-button"
            >
              {{ reconnecting ? '重新连接中...' : '重新连接' }}
            </el-button>
          </el-form-item>
          <el-form-item label=" " class="button-form-item">
            <el-button
              @click="handleSwitchAccount"
              class="full-width-button"
            >
              切换账号
            </el-button>
          </el-form-item>
        </template>

        <!-- 模式2：完整登录表单 -->
        <template v-else>
          <el-form-item label="服务器地址" prop="server">
            <el-input
              v-model="form.server"
              placeholder="例如: localhost:8080"
              :disabled="loading || autoFillMode"
            />
          </el-form-item>

          <el-form-item label="Client ID" prop="client">
            <el-input
              v-model="form.client"
              placeholder="用户名或邮箱"
              :disabled="loading || autoFillMode"
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

          <el-form-item label=" ">
            <el-checkbox v-model="form.rememberMe" :disabled="loading">
              记住登录
            </el-checkbox>
          </el-form-item>

          <el-form-item label=" " class="button-form-item">
            <el-button
              type="primary"
              :loading="loading"
              @click="handleLogin"
              class="full-width-button"
            >
              {{ loading ? '登录中...' : '登录' }}
            </el-button>
          </el-form-item>

          <el-form-item v-if="autoFillMode" label=" " class="button-form-item">
            <el-button
              @click="handleClearCredentials"
              class="full-width-button"
            >
              使用其他账号登录
            </el-button>
          </el-form-item>
        </template>
      </el-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { useAuthStore } from '../stores/auth'
import { Login, GetVersion, CheckSavedCredentials, ClearCredentials, CheckVersion } from '../../wailsjs/go/main/App'
import UpgradeDialog from '../components/UpgradeDialog.vue'

const router = useRouter()
const authStore = useAuthStore()
const appVersion = ref('dev')

const formRef = ref<FormInstance>()
const loading = ref(false)
const reconnecting = ref(false)
const loginMode = ref<'offline' | 'full'>('full')
const autoFillMode = ref(false)
const loginHint = ref('')
const isAutoLogging = ref(false)
const loadingDots = ref('')
const upgradeDialogRef = ref<InstanceType<typeof UpgradeDialog>>()

const form = reactive({
  server: authStore.serverAddress || 'localhost:8080',
  client: '',
  clientSecret: '',
  rememberMe: true
})

const rules: FormRules = {
  server: [
    { required: true, message: '请输入服务器地址', trigger: 'blur' }
  ],
  client: [
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
      // 先检查版本
      const versionCheck = await checkVersionBeforeLogin(form.server)
      if (!versionCheck) {
        loading.value = false
        return
      }

      await Login(
        form.server,
        form.client,
        form.clientSecret,
        form.rememberMe
      )

      authStore.setAuthenticated(true)
      authStore.setServerAddress(form.server)
      authStore.setClientId(form.client)

      ElMessage.success('登录成功')
      router.push('/services')
    } catch (error: any) {
      ElMessage.error(error.message || '登录失败')
    } finally {
      loading.value = false
    }
  })
}

const checkVersionBeforeLogin = async (serverAddr: string): Promise<boolean> => {
  try {
    // 构建完整的服务器地址（添加协议）
    let fullServerAddr = serverAddr
    if (!fullServerAddr.startsWith('http://') && !fullServerAddr.startsWith('https://')) {
      fullServerAddr = 'http://' + fullServerAddr
    }

    const versionInfo = await GetVersion()
    const versionCheckResult = await CheckVersion(fullServerAddr)
    
    if (!versionCheckResult.version_valid) {
      // 版本过低，显示升级对话框
      // 使用服务器地址 + /download 而不是 API 返回的 download_url
      const downloadPageUrl = fullServerAddr + '/download'
      upgradeDialogRef.value?.show(
        versionInfo.version,
        versionCheckResult.min_version,
        downloadPageUrl
      )
      return false
    }
    
    return true
  } catch (error: any) {
    console.error('Version check failed:', error)
    // 版本检查失败不阻止登录（可能是网络问题）
    return true
  }
}

const handleReconnect = async () => {
  reconnecting.value = true
  isAutoLogging.value = true
  startLoadingDots()
  
  try {
    // 先检查版本
    const versionCheck = await checkVersionBeforeLogin(form.server)
    if (!versionCheck) {
      reconnecting.value = false
      isAutoLogging.value = false
      stopLoadingDots()
      return
    }

    // 尝试使用保存的Token重新连接
    await Login(
      form.server,
      form.client,
      '', // 使用Token登录不需要Secret
      true
    )

    authStore.setAuthenticated(true)
    authStore.setServerAddress(form.server)
    authStore.setClientId(form.client)

    ElMessage.success('重新连接成功')
    router.push('/services')
  } catch (error: any) {
    ElMessage.error('重新连接失败: ' + (error.message || '未知错误'))
    // 切换到完整登录模式
    loginMode.value = 'full'
    autoFillMode.value = true
  } finally {
    reconnecting.value = false
    isAutoLogging.value = false
    stopLoadingDots()
  }
}

const handleSwitchAccount = () => {
  loginMode.value = 'full'
  autoFillMode.value = false
  form.clientSecret = ''
  loginHint.value = '请输入您的凭据以登录'
}

const handleClearCredentials = async () => {
  try {
    await ClearCredentials()
    form.server = 'localhost:8080'
    form.client = ''
    form.clientSecret = ''
    form.rememberMe = true
    autoFillMode.value = false
    loginHint.value = ''
    ElMessage.success('已清除保存的凭据')
  } catch (error: any) {
    ElMessage.error('清除凭据失败: ' + (error.message || '未知错误'))
  }
}

let dotsInterval: number | null = null

const startLoadingDots = () => {
  let count = 0
  loadingDots.value = ''
  dotsInterval = window.setInterval(() => {
    count = (count + 1) % 4
    loadingDots.value = '.'.repeat(count)
  }, 500)
}

const stopLoadingDots = () => {
  if (dotsInterval) {
    clearInterval(dotsInterval)
    dotsInterval = null
  }
  loadingDots.value = ''
}

const determineLoginMode = (savedCreds: any) => {
  if (!savedCreds) {
    loginMode.value = 'full'
    autoFillMode.value = false
    loginHint.value = '请输入您的凭据以登录'
    return
  }

  // 设置服务器地址（始终从后端获取，包括默认地址）
  form.server = savedCreds.server_address || 'localhost:8080'
  
  // 如果没有保存的用户信息，显示完整登录表单
  if (!savedCreds.client_id) {
    loginMode.value = 'full'
    autoFillMode.value = false
    loginHint.value = '请输入您的凭据以登录'
    return
  }

  // 有保存的凭据
  form.client = savedCreds.client_id
  form.rememberMe = savedCreds.remember_me

  // 检查是否有有效Token
  if (savedCreds.has_token && !savedCreds.is_online) {
    // 模式1：离线模式
    loginMode.value = 'offline'
    loginHint.value = ''
  } else if (savedCreds.has_token && savedCreds.is_online) {
    // 有Token且在线，尝试自动登录
    loginMode.value = 'full'
    autoFillMode.value = true
    isAutoLogging.value = true
    startLoadingDots()
    // 自动登录
    setTimeout(() => {
      handleReconnect()
    }, 500)
  } else {
    // 模式2：完整登录模式（自动填充）
    loginMode.value = 'full'
    autoFillMode.value = true
    loginHint.value = '欢迎回来！请输入密码以继续'
  }
}

onMounted(async () => {
  try {
    const versionInfo = await GetVersion()
    appVersion.value = versionInfo.version
  } catch (error) {
    console.error('Failed to get version:', error)
  }

  try {
    const savedCreds = await CheckSavedCredentials()
    determineLoginMode(savedCreds)
  } catch (error) {
    console.error('Failed to check saved credentials:', error)
    loginMode.value = 'full'
    autoFillMode.value = false
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
  width: 480px;
  max-width: 90vw;
}

.logo-container {
  text-align: center;
  margin-bottom: 20px;
}

.logo-wrapper {
  display: inline-block;
  position: relative;
  width: 80px;
  height: 80px;
}

.logo-wrapper.is-loading::before {
  content: '';
  position: absolute;
  top: -8px;
  left: -8px;
  right: -8px;
  bottom: -8px;
  border: 3px solid transparent;
  border-top-color: #409eff;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

.logo {
  width: 80px;
  height: 80px;
  object-fit: contain;
}

@keyframes spin {
  0% {
    transform: rotate(0deg);
  }
  100% {
    transform: rotate(360deg);
  }
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

.full-width-button {
  width: 100%;
}

.offline-alert {
  margin-bottom: 20px;
}

.offline-info {
  margin-top: 10px;
  font-size: 13px;
  line-height: 1.6;
}

.login-hint {
  margin-bottom: 20px;
}

/* 按钮表单项：移除label宽度，使按钮居中 */
.button-form-item {
  margin-bottom: 18px;
}

.button-form-item :deep(.el-form-item__label) {
  width: 0 !important;
  padding: 0 !important;
}

.button-form-item :deep(.el-form-item__content) {
  margin-left: 0 !important;
}
</style>
