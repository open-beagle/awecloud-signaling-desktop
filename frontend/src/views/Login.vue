<template>
  <div class="login-container">
    <UpgradeDialog ref="upgradeDialogRef" />
    <div class="login-box">
      <div class="logo-container">
        <div class="logo-wrapper" :class="{ 'is-loading': isAutoLogging || logtoLoading }">
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

      <!-- 系统默认浏览器等待授权状态 -->
      <el-alert
        v-else-if="logtoLoading && browserLoginURL"
        type="info"
        :closable="false"
        show-icon
        class="status-alert"
      >
        已在系统默认浏览器打开登录页面，完成授权后将自动获取身份进入客户端。
      </el-alert>

      <!-- 登录提示 -->
      <el-alert
        v-else-if="loginHint && !isAutoLogging"
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
            {{ logtoLoading ? '等待浏览器授权...' : '浏览器登录' }}
          </el-button>
        </el-form-item>

        <!-- 默认浏览器等待期间辅助按钮 -->
        <el-form-item v-if="logtoLoading && browserLoginURL" label=" " class="button-form-item">
          <div class="browser-wait-actions">
            <el-button @click="handleReopenBrowser" class="half-width-button">
              重新打开浏览器
            </el-button>
            <el-button @click="handleCancelLoginWait" class="half-width-button">
              取消等待
            </el-button>
          </div>
        </el-form-item>

        <!-- 切换账号按钮（仅在 saved 模式且未加载时显示） -->
        <el-form-item v-if="loginMode === 'saved' && !logtoLoading" label=" " class="button-form-item">
          <el-button
            @click="handleSwitchAccount"
            class="full-width-button"
            :disabled="isAutoLogging"
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
import { App } from '../../bindings/github.com/open-beagle/awecloud-signaling-desktop/internal/app'
import UpgradeDialog from '../components/UpgradeDialog.vue'

const {
  Login,
  GetVersion,
  CheckSavedCredentials,
  ClearCredentials,
  CreateLoginSession,
  OpenBrowser,
  CloseLoginWindow,
  WaitForLoginResultGRPC
} = App

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
const browserLoginURL = ref('')
const forcePromptLogin = ref(false)

const form = reactive({
  server: '',
  usernameHint: '',
  rememberMe: true
})

const validateServerAddress = (_rule: unknown, value: string, callback: (error?: Error) => void) => {
  try {
    const address = new URL(value.trim())
    if (!['http:', 'https:'].includes(address.protocol) || !address.hostname) {
      throw new Error('unsupported server address')
    }
    callback()
  } catch {
    callback(new Error('请输入有效的 HTTP(S) 服务器地址'))
  }
}

const rules: FormRules = {
  server: [
    { required: true, message: '请输入服务器地址', trigger: 'blur' },
    { validator: validateServerAddress, trigger: 'blur' }
  ]
}

const getSubtitle = () => {
  if (isAutoLogging.value) {
    return '正在自动登录...'
  }
  if (logtoLoading.value) {
    return '请在系统默认浏览器中完成认证'
  }
  if (loginMode.value === 'saved') {
    return `欢迎回来，${form.usernameHint}！`
  }
  return '连接到您的远程服务'
}

const buildBrowserLoginURL = (serverBase: string, loginPath: string, forcePrompt: boolean) => {
  const raw = `${serverBase}${loginPath}`
  if (!forcePrompt) {
    return raw
  }
  const sep = raw.includes('?') ? '&' : '?'
  return `${raw}${sep}prompt=login`
}

const handleReopenBrowser = async () => {
  if (!browserLoginURL.value) return
  try {
    await OpenBrowser(browserLoginURL.value)
    ElMessage.info('已重新在系统默认浏览器中打开登录页面')
  } catch (error: any) {
    ElMessage.error('打开系统默认浏览器失败: ' + (error.message || '未知错误'))
  }
}

const handleCancelLoginWait = async () => {
  try {
    await CloseLoginWindow()
  } catch {
    // ignore
  } finally {
    browserLoginURL.value = ''
    logtoLoading.value = false
    loginHint.value = '已取消等待，可重新点击「浏览器登录」'
  }
}

// 登录处理：调起系统默认浏览器并通过 gRPC 流等待用户身份回调
const handleLogin = async () => {
  if (!formRef.value) return

  await formRef.value.validate(async (valid) => {
    if (!valid) return

    if (!form.server) {
      ElMessage.warning('请输入服务器地址')
      return
    }

    form.server = form.server.trim().replace(/\/+$/, '')
    logtoLoading.value = true
    browserLoginURL.value = ''
    try {
      const sessionResult = await CreateLoginSession(form.server, form.usernameHint || '')

      if (!sessionResult || !sessionResult.session_id) {
        ElMessage.error('创建登录会话失败')
        logtoLoading.value = false
        return
      }

      const sessionIdFromResponse = sessionResult.session_id
      const fullLoginURL = buildBrowserLoginURL(
        form.server,
        sessionResult.login_url,
        forcePromptLogin.value
      )

      sessionId.value = sessionIdFromResponse
      browserLoginURL.value = fullLoginURL

      await OpenBrowser(fullLoginURL)

      const loginResult = await WaitForLoginResultGRPC(form.server, sessionIdFromResponse, '')

      if (!loginResult) {
        ElMessage.error('登录超时，请重试')
        browserLoginURL.value = ''
        logtoLoading.value = false
        return
      }

      if (loginResult.Success) {
        forcePromptLogin.value = false
        browserLoginURL.value = ''
        authStore.setServerAddress(form.server)
        authStore.setClientId(loginResult.Username || form.usernameHint || '')
        authStore.setAuthenticated(true)

        ElMessage.success('登录成功')
        await new Promise(resolve => setTimeout(resolve, 100))
        await router.push('/hosts')
      } else if (loginResult.IsDisabled) {
        browserLoginURL.value = ''
        logtoLoading.value = false
        const msg = loginResult.Message || '您的账号已被管理员禁用，请联系管理员'
        ElMessage.warning(msg)
        loginHint.value = msg
      } else {
        browserLoginURL.value = ''
        logtoLoading.value = false
        if (loginResult.Message !== '已取消登录等待') {
          ElMessage.error(loginResult.Message || '登录失败')
        }
      }
    } catch (error: any) {
      browserLoginURL.value = ''
      logtoLoading.value = false
      ElMessage.error('登录失败: ' + (error.message || '未知错误'))
    }
  })
}

// 切换账号：保留默认服务器地址并在下次浏览器登录附带 prompt=login
const handleSwitchAccount = async () => {
  try {
    const preservedServer = form.server
    await ClearCredentials()
    const freshCreds = await CheckSavedCredentials()
    form.server = freshCreds?.server_address || preservedServer
    form.usernameHint = ''
    form.rememberMe = true
    forcePromptLogin.value = true
    loginMode.value = 'new'
    loginHint.value = '已切换账号模式，点击「浏览器登录」将在默认浏览器中选择其他账号'
    ElMessage.success('已清除保存的凭据')
  } catch (error: any) {
    ElMessage.error('清除凭据失败: ' + (error.message || '未知错误'))
  }
}

const handleAutoLogin = async () => {
  try {
    await Login(
      form.server,
      form.usernameHint,
      '',
      true
    )

    authStore.setServerAddress(form.server)
    authStore.setClientId(form.usernameHint)
    authStore.setAuthenticated(true)

    ElMessage.success('自动登录成功')
    await new Promise(resolve => setTimeout(resolve, 100))
    await router.push('/hosts')
  } catch (error: any) {
    console.error('Auto login failed:', error)
    isAutoLogging.value = false
    loginMode.value = 'saved'

    const errorMsg = error?.message || error?.toString() || ''
    if (errorMsg.includes('禁用') || errorMsg.includes('disabled')) {
      loginHint.value = '您的账号已被禁用，请联系管理员'
    } else {
      loginHint.value = '自动登录失败，请点击「浏览器登录」继续'
    }
  }
}

const determineLoginMode = (savedCreds: any) => {
  if (!savedCreds) {
    loginMode.value = 'new'
    loginHint.value = ''
    return
  }

  form.server = savedCreds.server_address || ''

  if (!savedCreds.client_id) {
    loginMode.value = 'new'
    loginHint.value = ''
    return
  }

  form.usernameHint = savedCreds.client_id
  form.rememberMe = savedCreds.remember_me

  if (savedCreds.has_token) {
    loginMode.value = 'auto'
    isAutoLogging.value = true
    setTimeout(() => {
      handleAutoLogin()
    }, 500)
  } else {
    loginMode.value = 'saved'
    loginHint.value = '请点击「浏览器登录」继续'
  }
}

onMounted(async () => {
  if (authStore.isAuthenticated) {
    await router.push('/hosts')
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

.browser-wait-actions {
  display: flex;
  gap: 12px;
  width: 100%;
}

.half-width-button {
  flex: 1;
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
