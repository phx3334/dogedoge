import { defineStore } from 'pinia'
import { getMessageUnreadCount } from '@/api/message'

export const useMessageStore = defineStore('message', {
  state: () => ({
    unreadCount: 0,
  }),

  actions: {
    async fetchUnreadCount() {
      try {
        const res = await getMessageUnreadCount()
        this.unreadCount = res.count
      } catch {
        // 忽略未读数获取失败
      }
    },

    resetUnread() {
      this.unreadCount = 0
    },
  },
})
