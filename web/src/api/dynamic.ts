import request from './request'
import type { DynamicItem, PaginatedResp } from '@/types'

// 用户动态列表（仅图文动态）
export function getUserDynamics(user_id: string, page: number = 1, page_size: number = 20) {
  return request.get<any, PaginatedResp<DynamicItem>>('/dynamic/user', {
    params: { user_id, page, page_size },
  })
}

// 用户主页混合动态（视频+文章+图文动态）
export function getUserMixedDynamics(user_id: string, page: number = 1, page_size: number = 20) {
  return request.get<any, PaginatedResp<DynamicItem>>('/dynamic/user-mixed', {
    params: { user_id, page, page_size },
  })
}

// 动态 Feed 流
export function getDynamicFeed(page: number = 1, page_size: number = 20) {
  return request.get<any, PaginatedResp<DynamicItem>>('/dynamic/feed', {
    params: { page, page_size },
  })
}

// 发布动态
export function createDynamic(title: string, content: string, images: string[] = []) {
  return request.post<any, { dynamic_id: number }>('/dynamic/create', { title, content, images })
}

// 点赞动态
export function likeDynamic(dynamic_id: number) {
  return request.post<any, void>('/dynamic/like', { dynamic_id })
}

// 取消点赞动态
export function unlikeDynamic(dynamic_id: number) {
  return request.delete<any, void>('/dynamic/like', { params: { dynamic_id } })
}
