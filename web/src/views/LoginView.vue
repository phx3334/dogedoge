<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getCaptcha, sendEmailCode, login, register } from '@/api/user'
import { useUserStore } from '@/stores/user'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const mode = ref<'login' | 'register'>('login')
const email = ref('')
const password = ref('')
const username = ref('') // 注册时必填
const captchaInput = ref('')
const captchaId = ref('')
const captchaImage = ref('')
const emailCode = ref('')
const loading = ref(false)

async function refreshCaptcha() {
  try {
    const res: any = await getCaptcha()
    captchaId.value = res.captcha_id
    captchaImage.value = res.pic_path
  } catch {
    // 忽略
  }
}

async function handleSendEmailCode() {
  if (!email.value) return
  if (!captchaInput.value) return
  try {
    await sendEmailCode(email.value, captchaId.value, captchaInput.value)
    showToast('验证码已发送至邮箱')
  } catch {
    refreshCaptcha()
  }
}

async function handleSubmit() {
  if (!email.value || !password.value || !captchaInput.value) return
  if (mode.value === 'register' && (!username.value || !emailCode.value)) return
  loading.value = true
  try {
    if (mode.value === 'login') {
      const res: any = await login(email.value, password.value, captchaId.value, captchaInput.value)
      userStore.setToken(res.access_token, res.access_token_expire)
      userStore.setUserInfo(res.account)
      redirectAfterAuth()
    } else {
      await register({
        username: username.value,
        email: email.value,
        password: password.value,
        verifyCode: emailCode.value,
        captcha_id: captchaId.value,
        captcha: captchaInput.value,
      })
      mode.value = 'login'
      showToast('注册成功，请登录')
      refreshCaptcha()
    }
  } catch {
    refreshCaptcha()
  } finally {
    loading.value = false
  }
}

function redirectAfterAuth() {
  const redirect = route.query.redirect as string
  router.push(redirect || '/')
}

function switchMode() {
  mode.value = mode.value === 'login' ? 'register' : 'login'
  captchaInput.value = ''
  emailCode.value = ''
  username.value = ''
  refreshCaptcha()
}

// 简易 toast（避免引入 UI 库）
function showToast(msg: string) {
  if (typeof window === 'undefined') return
  const toast = document.createElement('div')
  toast.textContent = msg
  toast.style.cssText = 'position:fixed;top:20px;left:50%;transform:translateX(-50%);background:rgba(0,0,0,0.8);color:#fff;padding:8px 16px;border-radius:6px;z-index:9999;font-size:14px;box-shadow:0 2px 8px rgba(0,0,0,0.15)'
  document.body.appendChild(toast)
  setTimeout(() => toast.remove(), 3000)
}

onMounted(() => {
  refreshCaptcha()
})
</script>

<template>
  <div class="min-h-[calc(100vh-56px)] flex items-center justify-center px-4 py-8">
    <div class="w-full max-w-sm">
      <div class="bg-white rounded-xl shadow-card p-8">
        <!-- 标题 -->
        <div class="text-center mb-6">
          <h1 class="text-2xl font-bold text-ink">
            {{ mode === 'login' ? '登录' : '注册' }}
          </h1>
          <p class="text-sm text-ink-muted mt-1">仿B站视频社区</p>
        </div>

        <!-- 表单 -->
        <form class="space-y-4" @submit.prevent="handleSubmit">
          <!-- 用户名（注册时） -->
          <div v-if="mode === 'register'">
            <input
              v-model="username"
              type="text"
              placeholder="用户名（1-20 字符）"
              maxlength="20"
              class="w-full h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
            />
          </div>

          <!-- 邮箱 -->
          <div>
            <input
              v-model="email"
              type="email"
              placeholder="邮箱"
              class="w-full h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
            />
          </div>

          <!-- 密码（注册/登录统一 6-15 位） -->
          <div>
            <input
              v-model="password"
              type="password"
              placeholder="密码（6-15 位）"
              minlength="6"
              maxlength="15"
              class="w-full h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
            />
          </div>

          <!-- 图形验证码 -->
          <div class="flex gap-2">
            <input
              v-model="captchaInput"
              type="text"
              placeholder="图形验证码"
              maxlength="6"
              class="flex-1 h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
            />
            <img
              v-if="captchaImage"
              :src="captchaImage"
              alt="captcha"
              class="h-11 w-28 rounded-lg cursor-pointer border border-surface-muted"
              title="点击刷新"
              @click="refreshCaptcha"
            />
          </div>

          <!-- 邮箱验证码（注册时） -->
          <div v-if="mode === 'register'" class="flex gap-2">
            <input
              v-model="emailCode"
              type="text"
              placeholder="邮箱验证码"
              maxlength="6"
              class="flex-1 h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
            />
            <button
              type="button"
              class="px-3 h-11 rounded-lg text-sm text-secondary border border-secondary hover:bg-secondary/10 transition whitespace-nowrap"
              @click="handleSendEmailCode"
            >
              发送验证码
            </button>
          </div>

          <!-- 提交按钮 -->
          <button
            type="submit"
            :disabled="loading"
            class="w-full h-11 bg-primary text-white rounded-lg font-medium hover:bg-primary-dark transition disabled:opacity-60"
          >
            {{ loading ? '请稍候...' : (mode === 'login' ? '登录' : '注册') }}
          </button>
        </form>

        <!-- 切换 / 忘记密码 -->
        <div class="mt-6 flex items-center justify-between text-sm">
          <router-link to="/forgot-password" class="text-ink-muted hover:text-primary">
            忘记密码？
          </router-link>
          <span class="text-ink-muted">
            {{ mode === 'login' ? '没有账号？' : '已有账号？' }}
            <button class="text-primary hover:underline ml-1" @click="switchMode">
              {{ mode === 'login' ? '立即注册' : '去登录' }}
            </button>
          </span>
        </div>
      </div>
    </div>
  </div>
</template>
