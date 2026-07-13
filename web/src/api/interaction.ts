import request from './request'
import type { DanmakuItem } from '@/types'

// 点赞视频
export function likeVideo(video_id: number) {
  return request.post<any, void>('/interaction/video/like', { video_id })
}

// 取消点赞
export function unlikeVideo(video_id: number) {
  return request.post<any, void>('/interaction/video/unlike', { video_id })
}

// 收藏视频
export function favoriteVideo(video_id: number, folder_id: number = 0) {
  return request.post<any, void>('/interaction/video/favorite', { video_id, folder_id })
}

// 取消收藏
// folder_id 可选，0 或不传表示从所有收藏夹移除
export function unfavoriteVideo(video_id: number, folder_id: number = 0) {
  return request.post<any, void>('/interaction/video/unfavorite', { video_id, folder_id })
}

// 关注用户
// 后端 FollowUserReq 字段为 target_user_id（json:"target_user_id"）
export function followUser(target_user_id: string) {
  return request.post<any, void>('/interaction/follow', { target_user_id })
}

// 取消关注
export function unfollowUser(target_user_id: string) {
  return request.post<any, void>('/interaction/unfollow', { target_user_id })
}

// 获取弹幕
export function getDanmaku(video_id: number) {
  return request.get<any, DanmakuItem[]>('/interaction/video/danmaku', { params: { video_id } })
}

// 发送弹幕
export function sendDanmaku(data: {
  video_id: number
  content: string
  video_time: number
  color: string
  font_size: string
  mode: number // 0=滚动, 1=顶部, 2=底部
}) {
  return request.post<any, void>('/interaction/video/danmaku', data)
}
