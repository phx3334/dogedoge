import { defineStore } from 'pinia'
import type { Account } from '@/types'
import { logout as logoutApi, getUserInfo } from '@/api/user'

const TOKEN_KEY = 'fake_bili_token'
const USER_KEY = 'fake_bili_user'

export const useUserStore = defineStore('user', {
  state: () => ({
    token: localStorage.getItem(TOKEN_KEY) || '',
    userInfo: JSON.parse(localStorage.getItem(USER_KEY) || 'null') as Account | null,
  }),

  getters: {
    isLogin: (state) => !!state.token,
  },

  actions: {
    setToken(token: string, expire: number) {
      this.token = token
      localStorage.setItem(TOKEN_KEY, token)
    },

    setUserInfo(user: Account) {
      this.userInfo = user
      localStorage.setItem(USER_KEY, JSON.stringify(user))
    },

    // 页面加载时从后端拉取最新用户信息，确保头像等数据是最新的
    async fetchUserInfo() {
      if (!this.token) return
      try {
        const user = await getUserInfo()
        this.setUserInfo(user)
      } catch {
        // 如果 token 过期或请求失败，保持本地缓存不变
        // 不清除 token，让 JWT 刷新机制或后续请求自行处理
      }
    },

    async logout() {
      // 调用后端登出接口（拉黑 refresh token），即使失败也清除本地状态
      try {
        if (this.token) {
          await logoutApi()
        }
      } catch {
        // 忽略后端错误，本地一定清除
      }
      this.token = ''
      this.userInfo = null
      localStorage.removeItem(TOKEN_KEY)
      localStorage.removeItem(USER_KEY)
    },
  },
})
