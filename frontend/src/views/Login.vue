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
      <p class="subtitle">{{ getSubtitle() }}</p>

      <!-- 自动登录状态 -->
      <el-alert
        v-if="isAutoLogging"
        type="info"
        :closable="false"
        show-icon
        class="status-alert"
      >
        欢迎回来！正在使用保存的凭据登录...
      </el-alert>

      <!-- 登录提示 -->
      <el-alert
        v-if="loginHint && !isAutoLogging"
        type="info"
        :closable="false"
        class="status-alert"
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
        <!-- 服务器地址 -->
        <el-form-item label="服务器地址" prop="server">
          <el-input
            v-model="form.server"
            placeholder="例如: https://signal.example.com"
            :disabled="isAutoLogging || logtoLoading"
          />
        </el-form-item>

        <!-- 用户名提示（可选） -->
        <el-form-item label="用户名提示" prop="usernameHint">
          <el-input
            v-model="form.usernameHint"
            placeholder="可选，用于自动填充"
            :disabled="isAutoLogging || logtoLoading"
          />
        </el-form-item>

        <!-- 记住登录 -->
        <el-form-item label=" ">
          <el-checkbox v-model="form.rememberMe" :disabled="isAutoLogging || logtoLoading">
            记住登录
          </el-checkbox>
        </el-form-item>

        <!-- 登录按钮 -->
        <el-form-item label=" " class="button-form-item">
          <el-button
            type="primary"
            :loading="logtoLoading"
            @click="handleLogin"
            class="full-width-button"
            :disabled="isAutoLogging"
          >
            {{ logtoLoading ? '登录中...' : '登录' }}
          </el-button>
        </el-form-item>

        <!-- 切换账号按钮（仅在 saved 模式显示） -->
        <el-form-item v-if="loginMode === 'saved'" label=" " class="button-form-item">
          <el-button
            @click="handleSwitchAccount"
            class="full-width-button"
            :disabled="isAutoLogging || logtoLoading"
          >
            使用其他账号登录
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
import { App } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop'
import UpgradeDialog from '../components/UpgradeDialog.vue'

const { Login, GetVersion, CheckSavedCredentials, ClearCredentials, CreateLoginSession, OpenLoginWindow, WaitForLoginResultGRPC } = App

const router = useRouter()
const authStore = useAuthStore()
const appVersion = ref('dev')

const formRef = ref<FormInstance>()
const logtoLoading = ref(false)
const loginMode = ref<'auto' | 'saved' | 'new'>('new')
const loginHint = ref('')
const isAutoLogging = ref(false)
const upgradeDialogRef = ref<InstanceType<typeof UpgradeDialog>>()
const sessionId = ref('')

const form = reactive({
  server: '',
  usernameHint: '',
  rememberMe: true
})

const rules: FormRules = {
  server: [
    { required: true, message: '请输入服务器地址', trigger: 'blur' }
  ]
}

// 获取副标题文本
const getSubtitle = () => {
  if (isAutoLogging.value) {
    return '正在自动登录...'
  }
  if (loginMode.value === 'saved') {
    return `欢迎回来，${form.usernameHint}！`
  }
  return '连接到您的远程服务'
}

// 登录处理
const handleLogin = async () => {
  if (!formRef.value) return

  await formRef.value.validate(async (valid) => {
    if (!valid) return

    if (!form.server) {
      ElMessage.warning('请输入服务器地址')
      return
    }

    logtoLoading.value = true
    try {
      // 第一步：通过 gRPC 创建登录会话
      const sessionResult = await CreateLoginSession(form.server, form.usernameHint || '')
      
      if (!sessionResult || !sessionResult.session_id) {
        ElMessage.error('创建登录会话失败')
        logtoLoading.value = false
        return
      }

      const sessionIdFromResponse = sessionResult.session_id
      const loginURL = sessionResult.login_url

      // 保存 session_id
      sessionId.value = sessionIdFromResponse

      // 第二步：在 Desktop 内部的 WebView 窗口中打开登录页面
      ElMessage.info('正在打开登录窗口...')
      const fullLoginURL = form.server + loginURL
      await OpenLoginWindow(fullLoginURL)

      // 第三步：通过 gRPC 双向流等待登录完成（5 分钟超时）
      ElMessage.info('等待登录完成...')
      const loginResult = await WaitForLoginResultGRPC(form.server, sessionIdFromResponse, '')

      if (!loginResult) {
        ElMessage.error('登录超时，请重试')
        logtoLoading.value = false
        // 超时后返回登录界面，用户可以重新尝试
        return
      }

      if (loginResult.Success) {
        // 登录成功，保存凭证并设置认证状态
        authStore.setServerAddress(form.server)
        authStore.setClientId(loginResult.Username || form.usernameHint || '')
        authStore.setAuthenticated(true)

        ElMessage.success('登录成功')
        await new Promise(resolve => setTimeout(resolve, 100))
        await router.push('/services')
      } else if (loginResult.IsDisabled) {
        // 用户被禁用/待审批
        ElMessage.warning(loginResult.Message || '用户未注册或已禁用，请联系管理员审批')
        loginHint.value = loginResult.Message || '用户未注册或已禁用，请联系管理员审批'
        logtoLoading.value = false
      } else {
        ElMessage.error(loginResult.Message || '登录失败')
        logtoLoading.value = false
      }
    } catch (error: any) {
      ElMessage.error('登录失败: ' + (error.message || '未知错误'))
      logtoLoading.value = false
    }
  })
}

// 切换账号
const handleSwitchAccount = async () => {
  try {
    await ClearCredentials()
    form.server = ''
    form.usernameHint = ''
    form.rememberMe = true
    loginMode.value = 'new'
    loginHint.value = ''
    ElMessage.success('已清除保存的凭据')
  } catch (error: any) {
    ElMessage.error('清除凭据失败: ' + (error.message || '未知错误'))
  }
}

// 自动登录
const handleAutoLogin = async () => {
  try {
    // 尝试使用保存的Token自动登录
    await Login(
      form.server,
      form.usernameHint,
      '', // 使用Token登录不需要Secret
      true
    )

    // 先设置状态
    authStore.setServerAddress(form.server)
    authStore.setClientId(form.usernameHint)
    
    // 最后设置认证状态，触发路由守卫
    authStore.setAuthenticated(true)

    ElMessage.success('自动登录成功')
    
    // 延迟导航，确保状态已更新
    await new Promise(resolve => setTimeout(resolve, 100))
    
    // 导航到服务页面
    await router.push('/services')
  } catch (error: any) {
    console.error('Auto login failed:', error)
    // 自动登录失败，切换到 saved 模式
    isAutoLogging.value = false
    loginMode.value = 'saved'
    loginHint.value = '自动登录失败，请点击"登录"按钮重新登录'
  }
}

// 判断登录模式
const determineLoginMode = (savedCreds: any) => {
  if (!savedCreds) {
    loginMode.value = 'new'
    loginHint.value = ''
    return
  }

  // 设置服务器地址
  form.server = savedCreds.server_address || ''

  // 如果没有保存的用户信息，显示新登录界面
  if (!savedCreds.client_id) {
    loginMode.value = 'new'
    loginHint.value = ''
    return
  }

  // 有保存的凭据
  form.usernameHint = savedCreds.client_id
  form.rememberMe = savedCreds.remember_me

  // 检查是否有有效Token，有则尝试自动登录
  if (savedCreds.has_token) {
    loginMode.value = 'auto'
    isAutoLogging.value = true
    // 自动登录
    setTimeout(() => {
      handleAutoLogin()
    }, 500)
  } else {
    // 没有Token，显示 saved 模式
    loginMode.value = 'saved'
    loginHint.value = '请点击"登录"按钮继续'
  }
}

onMounted(async () => {
  // 如果已认证，直接导航到服务页面
  if (authStore.isAuthenticated) {
    await router.push('/services')
    return
  }

  try {
    const versionInfo = await GetVersion()
    appVersion.value = versionInfo?.version || 'dev'
  } catch (error) {
    console.error('Failed to get version:', error)
  }

  try {
    const savedCreds = await CheckSavedCredentials()
    determineLoginMode(savedCreds)
  } catch (error) {
    console.error('Failed to check saved credentials:', error)
    loginMode.value = 'new'
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

.status-alert {
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
