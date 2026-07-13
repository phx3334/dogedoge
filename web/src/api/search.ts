import request from './request'
import type { HomeVideoInfo } from '@/types'

// 搜索视频
export function searchVideo(keyword: string, cursor: string = '', limit: number = 20) {
  return request.get<any, { list: HomeVideoInfo[]; next_cursor: string; has_more: boolean }>(
    '/search/video',
    { params: { keyword, cursor, limit } }
  )
}
