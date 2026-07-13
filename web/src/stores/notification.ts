import { defineStore } from 'pinia'
import { getUnreadCount } from '@/api/notification'

export const useNotificationStore = defineStore('notification', {
  state: () => ({
    unreadCount: 0,
  }),

  actions: {
    async fetchUnreadCount() {
      try {
        const res = await getUnreadCount()
        this.unreadCount = res.count
      } catch {
        // 忽略未读数获取失败
      }
    },

    incrementUnread() {
      this.unreadCount++
    },

    resetUnread() {
      this.unreadCount = 0
    },
  },
})
