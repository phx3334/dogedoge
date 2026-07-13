<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { getCaptcha, sendEmailCode, forgotPassword } from '@/api/user'

const router = useRouter()

const email = ref('')
const captchaInput = ref('')
const captchaId = ref('')
const captchaImage = ref('')
const emailCode = ref('')
const newPassword = ref('')
const newPassword2 = ref('')
const sendingEmail = ref(false)
const submitting = ref(false)
const cooldown = ref(0)
let timer: any = null

function showToast(msg: string, ok = true) {
  if (typeof window === 'undefined') return
  const t = document.createElement('div')
  t.textContent = msg
  t.style.cssText = `position:fixed;top:20px;left:50%;transform:translateX(-50%);background:${ok ? 'rgba(0,0,0,.8)' : '#ff4d4f'};color:#fff;padding:8px 16px;border-radius:6px;z-index:9999;font-size:14px`
  document.body.appendChild(t)
  setTimeout(() => t.remove(), 2500)
}

async function refreshCaptcha() {
  try {
    const res: any = await getCaptcha()
    captchaId.value = res.captcha_id
    captchaImage.value = res.pic_path
  } catch {
    /* ignore */
  }
}

async function handleSendEmailCode() {
  if (!email.value) {
    showToast('请先输入邮箱', false)
    return
  }
  if (!captchaInput.value) {
    showToast('请先输入图形验证码', false)
    return
  }
  if (cooldown.value > 0) return
  sendingEmail.value = true
  try {
    await sendEmailCode(email.value, captchaId.value, captchaInput.value)
    showToast('验证码已发送至邮箱')
    cooldown.value = 60
    timer = setInterval(() => {
      cooldown.value--
      if (cooldown.value <= 0 && timer) {
        clearInterval(timer)
        timer = null
      }
    }, 1000)
  } catch {
    refreshCaptcha()
  } finally {
    sendingEmail.value = false
  }
}

async function handleSubmit() {
  if (!email.value || !emailCode.value || !newPassword.value) {
    showToast('请填写完整', false)
    return
  }
  if (newPassword.value.length < 6 || newPassword.value.length > 15) {
    showToast('新密码须为 6-15 位', false)
    return
  }
  if (newPassword.value !== newPassword2.value) {
    showToast('两次输入的密码不一致', false)
    return
  }
  submitting.value = true
  try {
    await forgotPassword({
      email: email.value,
      verifyCode: emailCode.value,
      newPassword: newPassword.value,
    })
    showToast('密码重置成功，请登录')
    setTimeout(() => router.push('/login'), 1500)
  } catch {
    refreshCaptcha()
  } finally {
    submitting.value = false
  }
}

onMounted(refreshCaptcha)
</script>

<template>
  <div class="min-h-[calc(100vh-56px)] flex items-center justify-center px-4 py-8">
    <div class="w-full max-w-sm">
      <div class="bg-white rounded-xl shadow-card p-8">
        <div class="text-center mb-6">
          <h1 class="text-2xl font-bold text-ink">找回密码</h1>
          <p class="text-sm text-ink-muted mt-1">通过邮箱验证码重置密码</p>
        </div>

        <form class="space-y-4" @submit.prevent="handleSubmit">
          <!-- 邮箱 -->
          <input
            v-model="email"
            type="email"
            placeholder="注册时使用的邮箱"
            class="w-full h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
          />

          <!-- 图形验证码 -->
          <div class="flex gap-2">
            <input
              v-model="captchaInput"
              type="text"
              placeholder="图形验证码"
              maxlength="6"
              class="flex-1 h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
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

          <!-- 邮箱验证码 -->
          <div class="flex gap-2">
            <input
              v-model="emailCode"
              type="text"
              placeholder="邮箱验证码"
              maxlength="6"
              class="flex-1 h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
            />
            <button
              type="button"
              :disabled="cooldown > 0 || sendingEmail"
              class="px-3 h-11 rounded-lg text-sm text-secondary border border-secondary hover:bg-secondary/10 transition whitespace-nowrap disabled:opacity-50"
              @click="handleSendEmailCode"
            >
              {{ cooldown > 0 ? `${cooldown}s 后重试` : '发送验证码' }}
            </button>
          </div>

          <!-- 新密码 -->
          <input
            v-model="newPassword"
            type="password"
            placeholder="新密码（6-15 位）"
            minlength="6"
            maxlength="15"
            class="w-full h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
          />
          <input
            v-model="newPassword2"
            type="password"
            placeholder="确认新密码"
            minlength="6"
            maxlength="15"
            class="w-full h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
          />

          <button
            type="submit"
            :disabled="submitting"
            class="w-full h-11 bg-primary text-white rounded-lg font-medium hover:bg-primary-dark transition disabled:opacity-60"
          >
            {{ submitting ? '重置中...' : '重置密码' }}
          </button>
        </form>

        <div class="mt-6 text-center text-sm text-ink-muted">
          想起来了？
          <router-link to="/login" class="text-primary hover:underline ml-1">返回登录</router-link>
        </div>
      </div>
    </div>
  </div>
</template>
