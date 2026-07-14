import request from './request'
import type { Account, LoginResp, UserLevelResp, CaptchaResp, UserHomeResp, UserBriefResp } from '@/types'

// 用户注册
// 后端 RegisterReq 字段：username / email / password / verifyCode（邮箱验证码）
// captcha_id / captcha（图形验证码） 来自 base
export function register(payload: { username: string; email: string; password: string; verifyCode: string; captcha_id: string; captcha: string }) {
  return request.post<any, Account>('/user/register', payload)
}

// 用户登录
export function login(email: string, password: string, captcha_id: string, captcha: string) {
  return request.post<any, LoginResp>('/user/login', {
    email,
    password,
    captcha_id,
    captcha,
  })
}

// 退出登录
export function logout() {
  return request.post<any, void>('/user/logout')
}

// 获取个人信息
export function getUserInfo() {
  return request.get<any, Account>('/user/info')
}

// 获取用户等级
export function getUserLevel() {
  return request.get<any, UserLevelResp>('/user/level')
}

// 修改个人信息
// 后端字段：username / signature / avatar / birthday / gender
export function changeUserInfo(payload: {
  username?: string
  signature?: string
  avatar?: string
  birthday?: string
  gender?: string
}) {
  return request.put<any, void>('/user/changeInfo', payload)
}

// 上传头像（multipart/form-data，字段名 avatar）
// 后端返回 { avatar_url: string }
// 注意：不要手动设置 Content-Type，axios 传 FormData 时会自动加上 boundary
export function uploadAvatar(file: File) {
  const fd = new FormData()
  fd.append('avatar', file)
  return request.post<any, { avatar_url: string }>('/user/avatar', fd, {
    timeout: 30000, // 头像上传给 30 秒
  })
}

// 通用图片上传（收藏夹封面等，multipart/form-data，字段名 file）
// 后端返回 { url: string }
export function uploadImage(file: File) {
  const fd = new FormData()
  fd.append('file', file)
  return request.post<any, { url: string }>('/user/upload/image', fd, {
    timeout: 30000,
  })
}

// 找回密码
export function forgotPassword(payload: {
  email: string
  verifyCode: string
  newPassword: string
}) {
  return request.post<any, void>('/user/forgotPassword', payload)
}

// 发送邮箱验证码
export function sendEmailCode(email: string, captcha_id: string, captcha: string) {
  return request.post<any, void>('/base/sendEmainCode', {
    email,
    captcha_id,
    captcha,
  })
}

// 获取图形验证码
export function getCaptcha() {
  return request.post<any, CaptchaResp>('/base/captcha')
}

// 获取用户主页信息（含视频列表、收藏夹、统计数据）
export function getUserHome(user_id: string, page: number = 1, page_size: number = 20) {
  return request.get<any, UserHomeResp>('/user/home', { params: { user_id, page, page_size } })
}

// 获取用户简档（供私信入口按 user_id 拉取对端资料，无需手动输入 ID）
export function getUserBrief(user_id: string) {
  return request.get<any, UserBriefResp>('/user/brief', { params: { user_id } })
}
