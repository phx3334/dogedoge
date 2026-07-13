import request from './request'
import type { ConversationItem, MessageItem, MessageUnreadResp, PaginatedResp } from '@/types'

// 会话列表
export function getConversations(page: number = 1, pageSize: number = 20) {
  return request.get<any, PaginatedResp<ConversationItem>>('/message/conversations', {
    params: { page, page_size: pageSize },
  })
}

// 与某对端的私信历史
export function getMessageHistory(peerId: string, page: number = 1, pageSize: number = 30) {
  return request.get<any, PaginatedResp<MessageItem>>('/message/history', {
    params: { peer_id: peerId, page, page_size: pageSize },
  })
}

// 发送私信
export function sendMessage(recipientId: string, content: string) {
  return request.post<any, MessageItem>('/message/send', {
    recipient_id: recipientId,
    content,
  })
}

// 标记与某对端的私信为已读
export function markMessageRead(peerId: string) {
  return request.post<any, void>('/message/read', { peer_id: peerId })
}

// 私信未读数
export function getMessageUnreadCount() {
  return request.get<any, MessageUnreadResp>('/message/unread_count')
}
