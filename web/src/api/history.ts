import request from './request'
import type { VideoHistoryItem, ArticleHistoryItem, SearchHistoryItem, PaginatedResp } from '@/types'

// 视频观看历史列表
export function getVideoHistory(page: number = 1, page_size: number = 20) {
  return request.get<any, PaginatedResp<VideoHistoryItem>>('/history/video/list', {
    params: { page, page_size },
  })
}

// 记录视频观看进度
export function recordVideoView(
  video_id: number,
  progress_sec: number,
  duration_sec: number,
  device: string = 'web'
) {
  return request.post<any, void>('/history/video/view', {
    video_id,
    progress_sec,
    duration_sec,
    device,
  })
}

// 删除单条视频观看历史
export function deleteVideoHistory(video_id: number) {
  return request.delete<any, void>('/history/video', { data: { video_id } })
}

// 清空视频观看历史
export function clearVideoHistory() {
  return request.post<any, void>('/history/video/clear')
}

// 文章阅读历史列表
export function getArticleHistory(page: number = 1, page_size: number = 20) {
  return request.get<any, PaginatedResp<ArticleHistoryItem>>('/history/article/list', {
    params: { page, page_size },
  })
}

// 删除单条文章阅读历史
export function deleteArticleHistory(article_id: number) {
  return request.delete<any, void>('/history/article', { data: { article_id } })
}

// 搜索历史列表
export function getSearchHistory(limit: number = 20) {
  return request.get<any, SearchHistoryItem[]>('/history/search/list', { params: { limit } })
}

// 保存搜索历史
export function saveSearchHistory(keyword: string) {
  return request.post<any, void>('/history/search', { keyword })
}

// 删除单条搜索历史
export function deleteSearchHistory(keyword: string) {
  return request.delete<any, void>('/history/search', { data: { keyword } })
}

// 清空搜索历史
export function clearSearchHistory() {
  return request.post<any, void>('/history/search/clear')
}
