import request from './request'
import type { DailyTaskResp } from '@/types'

// 触发每日登录奖励
export function triggerDailyLogin() {
  return request.post<any, { rewarded: boolean }>('/daily/login')
}

// 触发每日观看奖励（进入视频详情页时调用，后端按天幂等）
export function triggerDailyWatch() {
  return request.post<any, { rewarded: boolean }>('/daily/watch')
}

// 同一次会话内只触发一次观看任务（后端按天幂等，这里仅减少请求）
let watchTriggeredThisSession = false
export function triggerDailyWatchOnce() {
  if (watchTriggeredThisSession) return
  watchTriggeredThisSession = true
  triggerDailyWatch().catch(() => {})
}

// 获取今日任务完成情况
export function getDailyTask() {
  return request.get<any, DailyTaskResp>('/daily/today')
}
