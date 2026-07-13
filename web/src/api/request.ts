import axios, { type AxiosRequestConfig, type AxiosResponse } from 'axios'
import type { ApiResponse } from '@/types'

const TOKEN_KEY = 'fake_bili_token'
const USER_KEY = 'fake_bili_user'

const request = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
  withCredentials: true, // 携带 cookie（后端 refresh token 通过 x-refresh-token cookie 传递）
})

// 请求拦截器：使用 x-access-token 头传递 access token
// 注意：后端 pkg.GetAccessToken 读取的是 "x-access-token" 头，不是 Authorization
request.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem(TOKEN_KEY)
    if (token) {
      config.headers['x-access-token'] = token
    }
    return config
  },
  (error) => Promise.reject(error)
)

// 响应拦截器：统一处理业务错误 + 自动刷新 token
request.interceptors.response.use(
  (response: AxiosResponse<ApiResponse>) => {
    // 后端 JWT 中间件在 access token 过期时会通过 refresh token 续签，
    // 并在响应头返回 new-access-token / new-access-expiry
    const newToken = response.headers['new-access-token']
    if (newToken) {
      localStorage.setItem(TOKEN_KEY, newToken)
    }

    const res = response.data
    // code 3 = 成功
    if (res.code === 3) {
      // 后端在列表为空时返回 list: null，前端统一归一化为 []，避免 .length / .filter 等崩溃
      const data = res.data
      if (data && typeof data === 'object' && 'list' in data && data.list === null) {
        data.list = []
      }
      return data as any
    }
    // 鉴权失败：后端返回 code=4, data={ reload: true }，HTTP 状态码恒为 200
    if (res.data && typeof res.data === 'object' && (res.data as any).reload === true) {
      localStorage.removeItem(TOKEN_KEY)
      localStorage.removeItem(USER_KEY)
      const redirect = window.location.pathname + window.location.search
      window.location.href = `/login?redirect=${encodeURIComponent(redirect)}`
      return Promise.reject(new Error('登录已过期，请重新登录'))
    }
    // 业务错误
    showToast(res.msg || '请求失败')
    return Promise.reject(new Error(res.msg || '请求失败'))
  },
  (error) => {
    if (error.response?.status === 401) {
      // 兜底：部分网关/代理可能返回 401
      localStorage.removeItem(TOKEN_KEY)
      localStorage.removeItem(USER_KEY)
      const redirect = window.location.pathname + window.location.search
      window.location.href = `/login?redirect=${encodeURIComponent(redirect)}`
      return Promise.reject(new Error('登录已过期，请重新登录'))
    }
    const msg = error.response?.data?.msg || error.message || '网络异常'
    showToast(msg)
    return Promise.reject(new Error(msg))
  }
)

// 简易消息提示（避免引入完整 UI 库，用原生实现）
function showToast(msg: string) {
  console.warn('[API Error]', msg)
  if (typeof window !== 'undefined') {
    const toast = document.createElement('div')
    toast.textContent = msg
    toast.style.cssText = 'position:fixed;top:20px;left:50%;transform:translateX(-50%);background:#ff4d4f;color:#fff;padding:8px 16px;border-radius:6px;z-index:9999;font-size:14px;box-shadow:0 2px 8px rgba(0,0,0,0.15)'
    document.body.appendChild(toast)
    setTimeout(() => toast.remove(), 3000)
  }
}

export default request
export type { AxiosRequestConfig }
