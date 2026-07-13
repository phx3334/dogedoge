import request from './request'
import type { FavoriteFolder, HomeVideoInfo, PaginatedResp } from '@/types'

// 收藏夹列表
export function getFavoriteFolders() {
  return request.get<any, FavoriteFolder[]>('/favorite/folders')
}

// 创建收藏夹
export function createFavoriteFolder(title: string, cover_url?: string) {
  return request.post<any, { folder_id: number }>('/favorite/folder', { title, cover_url })
}

// 更新收藏夹
export function updateFavoriteFolder(folder_id: number, title: string, cover_url?: string) {
  return request.put<any, void>('/favorite/folder', { folder_id, title, cover_url })
}

// 删除收藏夹
export function deleteFavoriteFolder(folder_id: number) {
  return request.delete<any, void>('/favorite/folder', { data: { folder_id } })
}

// 收藏夹内视频列表
export function getFolderVideos(folder_id: number, page: number = 1, page_size: number = 20) {
  return request.get<any, PaginatedResp<HomeVideoInfo>>('/favorite/folder/videos', {
    params: { folder_id, page, page_size },
  })
}

// 移动收藏到指定收藏夹
export function moveFavorite(video_id: number, folder_id: number) {
  return request.post<any, void>('/favorite/move', { video_id, folder_id })
}
