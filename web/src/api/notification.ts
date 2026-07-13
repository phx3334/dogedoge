import request from './request'
import type { NotificationItem, UnreadCountResp, PaginatedResp } from '@/types'

// 通知列表
export function getNotifications(
  page: number = 1,
  page_size: number = 20,
  type?: string,
  only_unread?: boolean
) {
  return request.get<any, PaginatedResp<NotificationItem>>('/notification/list', {
    params: { page, page_size, type, only_unread },
  })
}

// 未读数
export function getUnreadCount() {
  return request.get<any, UnreadCountResp>('/notification/unread_count')
}

// 标记已读
export function markNotificationRead(notification_id: number) {
  return request.post<any, void>('/notification/read', { notification_id })
}

// 全部已读
export function markAllNotificationsRead() {
  return request.post<any, void>('/notification/read_all')
}

// 静默评论点赞通知
export function muteLikeNotification(comment_id: number) {
  return request.post<any, void>('/notification/mute_like', { comment_id })
}

// 删除单条通知
export function deleteNotification(notification_id: number) {
  return request.post<any, void>('/notification/delete', { notification_id })
}
