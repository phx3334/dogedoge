<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { CheckCheck, Bell, MessageSquare, Send, Trash2, Bot, ArrowLeft } from 'lucide-vue-next'
import { getNotifications, markNotificationRead, markAllNotificationsRead, deleteNotification } from '@/api/notification'
import { getConversations, getMessageHistory, sendMessage, markMessageRead } from '@/api/message'
import { getAICharacters, chatWithAIStream, type AICharacter, type ChatMessage } from '@/api/ai'
import { useNotificationStore } from '@/stores/notification'
import { useMessageStore } from '@/stores/message'
import { useUserStore } from '@/stores/user'
import type { NotificationItem, ConversationItem, MessageItem, PaginatedResp } from '@/types'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const notifStore = useNotificationStore()
const messageStore = useMessageStore()
const userStore = useUserStore()

const activeTab = ref<'notif' | 'message' | 'ai'>('notif')

// ============================ 通知 ============================
const list = ref<NotificationItem[]>([])
const page = ref(1)
const total = ref(0)
const pageSize = 15
const loading = ref(false)
const unreadCount = computed(() => list.value.filter((n) => !n.is_read).length)

function fmtDate(d: string) {
  try {
    return new Date(d).toLocaleDateString()
  } catch {
    return d
  }
}

// 解析 sender_names_json + payload_json 提取展示字段与跳转字段
function parseItem(item: NotificationItem): NotificationItem {
  const parsed: NotificationItem = { ...item }
  try {
    const names = JSON.parse(item.sender_names_json || '[]')
    parsed.sender_name = Array.isArray(names) && names.length ? names[0] : '系统通知'
  } catch {
    parsed.sender_name = '系统通知'
  }
  try {
    const p = JSON.parse(item.payload_json || '{}')
    parsed.content = p.content || item.comment_preview || ''
    parsed.sender_avatar = p.sender_avatar || ''
    parsed.target_type = p.target_type || ''
    parsed.target_id = p.target_id || ''
    parsed.comment_id = p.comment_id || ''
  } catch {
    parsed.content = item.comment_preview || ''
  }
  return parsed
}

async function loadNotif() {
  loading.value = true
  try {
    const res: PaginatedResp<NotificationItem> = await getNotifications(page.value, pageSize)
    list.value = (res.list || []).map(parseItem)
    total.value = res.total
  } finally {
    loading.value = false
  }
}

async function onRead(item: NotificationItem) {
  if (item.is_read) return
  await markNotificationRead(item.id)
  item.is_read = true
  notifStore.fetchUnreadCount()
}

async function onReadAll() {
  if (!list.value.some((n) => !n.is_read)) return
  await markAllNotificationsRead()
  list.value.forEach((n) => (n.is_read = true))
  notifStore.fetchUnreadCount()
}

// 删除单条通知
async function onDelete(item: NotificationItem) {
  try {
    await deleteNotification(item.id)
    list.value = list.value.filter((n) => n.id !== item.id)
    if (total.value > 0) total.value -= 1
    notifStore.fetchUnreadCount()
  } catch (err) {
    console.error('删除通知失败:', err)
  }
}

// 点击评论类通知 → 跳转对应评论
function onNotifClick(item: NotificationItem) {
  const isComment = item.type === 'reply_received' || item.type === 'comment_like'
  onRead(item)
  if (isComment && item.target_type && item.comment_id) {
    if (item.target_type === 'video') {
      router.push(`/video/${item.target_id}?comment_id=${item.comment_id}`)
    } else if (item.target_type === 'article') {
      router.push(`/article/${item.target_id}?comment_id=${item.comment_id}`)
    } else {
      router.push('/dynamic')
    }
  }
}

async function onPageChange(p: number) {
  page.value = p
  await loadNotif()
  window.scrollTo({ top: 0 })
}

function notifLabel(type: string) {
  if (type === 'comment_like') return '赞了你的评论'
  if (type === 'reply_received') return '回复了你'
  return '通知'
}

// ============================ 私信 ============================
const conversations = ref<ConversationItem[]>([])
const convLoading = ref(false)
const selectedPeer = ref<string>('')
const peerName = ref('')
const peerAvatar = ref('')
const messages = ref<MessageItem[]>([])
const msgLoading = ref(false)
const draft = ref('')
const sending = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null

async function loadConversations() {
  convLoading.value = true
  try {
    const res = await getConversations(1, 50)
    conversations.value = res.list || []
  } catch {
    conversations.value = []
  } finally {
    convLoading.value = false
  }
}

function scrollToBottom() {
  setTimeout(() => {
    const el = document.getElementById('msg-thread')
    if (el) el.scrollTop = el.scrollHeight
  }, 50)
}

async function loadHistory() {
  if (!selectedPeer.value) return
  msgLoading.value = true
  try {
    const res = await getMessageHistory(selectedPeer.value, 1, 100)
    messages.value = res.list || []
    scrollToBottom()
  } finally {
    msgLoading.value = false
  }
}

async function selectPeer(peerId: string, name = '', avatar = '') {
  selectedPeer.value = peerId
  peerName.value = name
  peerAvatar.value = avatar
  await loadHistory()
  // 标记已读 + 刷新未读/会话列表
  try {
    await markMessageRead(peerId)
  } catch {}
  messageStore.fetchUnreadCount()
  loadConversations()
  startPoll()
}

async function send() {
  const content = draft.value.trim()
  if (!content || !selectedPeer.value || sending.value) return
  sending.value = true
  try {
    await sendMessage(selectedPeer.value, content)
    draft.value = ''
    await loadHistory()
  } finally {
    sending.value = false
  }
}

function startPoll() {
  stopPoll()
  pollTimer = setInterval(async () => {
    if (!selectedPeer.value) return
    try {
      const res = await getMessageHistory(selectedPeer.value, 1, 100)
      messages.value = res.list || []
      scrollToBottom()
      await markMessageRead(selectedPeer.value)
      messageStore.fetchUnreadCount()
    } catch {}
  }, 3000)
}

function stopPoll() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

function startNewChat() {
  const pid = window.prompt('输入对方的用户 ID 发起私信：')
  if (!pid) return
  selectedPeer.value = pid
  peerName.value = ''
  peerAvatar.value = ''
  messages.value = []
  startPoll()
}

// ============================ AI 聊天 ============================
const aiCharacters = ref<AICharacter[]>([])
const activeAI = ref<AICharacter | null>(null)
const aiHistories = ref<Record<string, ChatMessage[]>>({})
const aiInput = ref('')
const aiSending = ref(false)
const aiEnd = ref<HTMLDivElement | null>(null)
const AI_STORAGE_KEY = 'fake_doge_ai_chat_history'

function loadAIHistories() {
  try {
    const raw = localStorage.getItem(AI_STORAGE_KEY)
    if (raw) aiHistories.value = JSON.parse(raw)
  } catch {
    aiHistories.value = {}
  }
}

function saveAIHistories() {
  try {
    localStorage.setItem(AI_STORAGE_KEY, JSON.stringify(aiHistories.value))
  } catch {
    // ignore
  }
}

function aiCurrentMessages(): ChatMessage[] {
  return activeAI.value ? aiHistories.value[activeAI.value.id] || [] : []
}

async function scrollAIToBottom() {
  await nextTick()
  aiEnd.value?.scrollIntoView({ behavior: 'smooth' })
}

function selectAI(char: AICharacter) {
  activeAI.value = char
  scrollAIToBottom()
}

function backToAIList() {
  activeAI.value = null
}

async function sendAIMessage() {
  if (!activeAI.value || !aiInput.value.trim() || aiSending.value) return

  const text = aiInput.value.trim()
  aiInput.value = ''

  const charId = activeAI.value.id
  if (!aiHistories.value[charId]) aiHistories.value[charId] = []

  const history = aiHistories.value[charId]
  history.push({ role: 'user', content: text })
  const assistantIndex = history.length
  history.push({ role: 'assistant', content: '' })
  saveAIHistories()
  scrollAIToBottom()

  aiSending.value = true
  try {
    const payload = [...history.slice(0, assistantIndex)]
    await chatWithAIStream(charId, payload, (chunk) => {
      history[assistantIndex].content += chunk
      saveAIHistories()
      scrollAIToBottom()
    })
    if (!history[assistantIndex].content) {
      history[assistantIndex].content = '（暂时没有回复）'
    }
  } catch (err) {
    history.pop()
    history.pop()
    saveAIHistories()
    console.error('AI 聊天失败:', err)
  } finally {
    aiSending.value = false
  }
}

function onAIKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    sendAIMessage()
  }
}

function clearAIHistory() {
  if (!activeAI.value) return
  if (!confirm(`确定要清空与 ${activeAI.value.name} 的聊天记录吗？`)) return
  aiHistories.value[activeAI.value.id] = []
  saveAIHistories()
}

// ============================ Tab 切换 ============================
async function switchTab(tab: 'notif' | 'message' | 'ai') {
  activeTab.value = tab
  if (tab === 'message') {
    await loadConversations()
    const peer = route.query.peer as string
    if (peer) {
      const conv = conversations.value.find((c) => c.peer_id === peer)
      await selectPeer(peer, conv?.peer_name, conv?.peer_avatar)
    }
  } else if (tab === 'ai') {
    stopPoll()
    if (!aiCharacters.value.length) {
      try {
        const res = await getAICharacters()
        aiCharacters.value = res.list || []
      } catch (err) {
        console.error('获取AI角色列表失败:', err)
      }
    }
  } else {
    stopPoll()
  }
}

onMounted(() => {
  loadNotif()
  loadAIHistories()
  if (route.query.tab === 'message') {
    switchTab('message')
  }
})

onUnmounted(() => stopPoll())
</script>

<template>
  <div class="space-y-4">
    <!-- Tab 栏 -->
    <div class="flex items-center gap-1 border-b border-surface-muted">
      <button
        class="relative flex items-center gap-1.5 px-3 pb-3 text-sm transition"
        :class="activeTab === 'notif' ? 'text-primary font-medium' : 'text-ink-secondary hover:text-ink'"
        @click="switchTab('notif')"
      >
        <Bell :size="16" />
        通知
        <span
          v-if="notifStore.unreadCount > 0"
          class="min-w-[16px] h-4 px-1 bg-primary/10 text-primary text-[10px] leading-4 rounded-full text-center"
        >{{ notifStore.unreadCount > 99 ? '99+' : notifStore.unreadCount }}</span>
      </button>
      <button
        class="relative flex items-center gap-1.5 px-3 pb-3 text-sm transition"
        :class="activeTab === 'message' ? 'text-primary font-medium' : 'text-ink-secondary hover:text-ink'"
        @click="switchTab('message')"
      >
        <MessageSquare :size="16" />
        私信
        <span
          v-if="messageStore.unreadCount > 0"
          class="min-w-[16px] h-4 px-1 bg-primary/10 text-primary text-[10px] leading-4 rounded-full text-center"
        >{{ messageStore.unreadCount > 99 ? '99+' : messageStore.unreadCount }}</span>
      </button>
      <button
        class="relative flex items-center gap-1.5 px-3 pb-3 text-sm transition"
        :class="activeTab === 'ai' ? 'text-primary font-medium' : 'text-ink-secondary hover:text-ink'"
        @click="switchTab('ai')"
      >
        <Bot :size="16" />
        AI 助手
      </button>
    </div>

    <!-- 通知列表 -->
    <div v-if="activeTab === 'notif'" class="bg-white rounded-card shadow-card p-4">
      <div class="flex items-center justify-between mb-2">
        <h2 class="text-lg font-bold text-ink">消息通知</h2>
        <button
          class="flex items-center gap-1 px-3 h-9 rounded-card text-sm transition"
          :class="unreadCount > 0
            ? 'bg-primary text-white hover:bg-primary-dark'
            : 'bg-surface-subtle text-ink-muted cursor-not-allowed'"
          :disabled="unreadCount === 0"
          @click="onReadAll"
        >
          <CheckCheck :size="16" />
          全部已读
        </button>
      </div>

      <div v-if="loading" class="py-12 text-center text-sm text-ink-muted">加载中...</div>
      <template v-else-if="list.length">
        <div class="space-y-1">
          <div
            v-for="item in list"
            :key="item.id"
            class="relative flex items-start gap-3 p-3 rounded transition cursor-pointer hover:bg-surface-subtle"
            :class="!item.is_read ? 'bg-primary/5' : ''"
            @click="onNotifClick(item)"
          >
            <span
              v-if="!item.is_read"
              class="absolute left-1 top-1/2 -translate-y-1/2 w-1.5 h-1.5 bg-primary rounded-full"
            ></span>
            <button
              class="absolute right-2 top-2 p-1 rounded text-ink-muted hover:text-red-500 hover:bg-red-50 transition"
              title="删除通知"
              @click.stop="onDelete(item)"
            >
              <Trash2 :size="15" />
            </button>
            <div class="w-9 h-9 rounded-full bg-surface-muted overflow-hidden shrink-0">
              <img
                :src="item.sender_avatar || '/uploads/avatar/default.jpg'"
                :alt="item.sender_name"
                class="w-full h-full object-cover"
                @error="($event.target as HTMLImageElement).src = '/uploads/avatar/default.jpg'"
              />
            </div>
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium text-ink truncate">{{ item.sender_name }}</span>
                <span
                  v-if="item.type === 'comment_like' || item.type === 'reply_received'"
                  class="text-[10px] text-primary bg-primary/10 px-1 rounded"
                >{{ notifLabel(item.type) }}</span>
                <span class="text-xs text-ink-muted">{{ fmtDate(item.created_at) }}</span>
              </div>
              <p class="text-sm text-ink-secondary mt-1 break-words">{{ item.content }}</p>
            </div>
          </div>
        </div>
        <Pagination :current="page" :total="total" :page-size="pageSize" @change="onPageChange" />
      </template>
      <EmptyState v-else text="暂无消息通知" />
    </div>

    <!-- 私信 -->
    <div v-else-if="activeTab === 'message'" class="bg-white rounded-card shadow-card flex h-[72vh] overflow-hidden">
      <!-- 会话列表 -->
      <div class="w-72 shrink-0 border-r border-surface-subtle flex flex-col">
        <div class="p-3 border-b border-surface-subtle">
          <button
            class="w-full flex items-center justify-center gap-1 px-3 h-9 rounded-card bg-primary/10 text-primary text-sm hover:bg-primary/20 transition"
            @click="startNewChat"
          >
            ＋ 发起私信
          </button>
        </div>
        <div class="flex-1 overflow-y-auto">
          <div
            v-for="c in conversations"
            :key="c.peer_id"
            class="flex items-center gap-3 p-3 border-b border-surface-subtle cursor-pointer transition hover:bg-surface-subtle"
            :class="selectedPeer === c.peer_id ? 'bg-primary/5' : ''"
            @click="selectPeer(c.peer_id, c.peer_name, c.peer_avatar)"
          >
            <div class="w-10 h-10 rounded-full bg-surface-muted overflow-hidden shrink-0">
              <img
                :src="c.peer_avatar || '/uploads/avatar/default.jpg'"
                :alt="c.peer_name"
                class="w-full h-full object-cover"
                @error="($event.target as HTMLImageElement).src = '/uploads/avatar/default.jpg'"
              />
            </div>
            <div class="flex-1 min-w-0">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-ink truncate">{{ c.peer_name || '用户' }}</span>
                <span
                  v-if="c.unread_count > 0"
                  class="min-w-[16px] h-4 px-1 bg-primary text-white text-[10px] leading-4 rounded-full text-center shrink-0"
                >{{ c.unread_count > 99 ? '99+' : c.unread_count }}</span>
              </div>
              <p class="text-xs text-ink-muted truncate mt-0.5">{{ c.last_content || '暂无消息' }}</p>
            </div>
          </div>
          <EmptyState v-if="!conversations.length && !convLoading" text="还没有会话" />
        </div>
      </div>

      <!-- 聊天窗口 -->
      <div class="flex-1 flex flex-col min-w-0">
        <template v-if="selectedPeer">
          <div class="h-14 px-4 flex items-center gap-2 border-b border-surface-subtle shrink-0">
            <div class="w-8 h-8 rounded-full bg-surface-muted overflow-hidden">
              <img
                :src="peerAvatar || '/uploads/avatar/default.jpg'"
                :alt="peerName"
                class="w-full h-full object-cover"
                @error="($event.target as HTMLImageElement).src = '/uploads/avatar/default.jpg'"
              />
            </div>
            <span class="text-sm font-medium text-ink">{{ peerName || '用户' }}</span>
          </div>

          <div id="msg-thread" class="flex-1 overflow-y-auto p-4 space-y-3 bg-surface-subtle/40">
            <div
              v-for="m in messages"
              :key="m.id"
              class="flex"
              :class="m.sender_id === userStore.userInfo?.id ? 'justify-end' : 'justify-start'"
            >
              <div
                class="max-w-[70%] px-3 py-2 rounded-2xl text-sm break-words"
                :class="m.sender_id === userStore.userInfo?.id
                  ? 'bg-primary text-white rounded-tr-sm'
                  : 'bg-white text-ink rounded-tl-sm shadow-sm'"
              >
                {{ m.content }}
              </div>
            </div>
            <div v-if="!messages.length && !msgLoading" class="text-center text-xs text-ink-muted py-8">
              还没有消息，发送第一条吧～
            </div>
          </div>

          <div class="p-3 border-t border-surface-subtle shrink-0 flex items-end gap-2">
            <textarea
              v-model="draft"
              rows="2"
              placeholder="输入私信内容，回车发送"
              class="flex-1 px-3 py-2 bg-surface-subtle rounded-card text-sm resize-none focus:outline-none focus:ring-2 focus:ring-primary/30"
              @keydown.enter.exact.prevent="send"
            ></textarea>
            <button
              class="px-4 h-9 rounded-card bg-primary text-white text-sm hover:bg-primary-dark transition disabled:opacity-50"
              :disabled="!draft.trim() || sending"
              @click="send"
            >
              <Send :size="16" />
            </button>
          </div>
        </template>
        <EmptyState v-else text="选择一个会话开始聊天，或从他人主页发起私信" />
      </div>
    </div>

    <!-- AI 助手 -->
    <div v-else-if="activeTab === 'ai'" class="space-y-4">
      <!-- 角色选择 -->
      <div v-if="!activeAI" class="bg-white rounded-card shadow-card p-4">
        <h2 class="text-lg font-bold text-ink mb-4">AI 助手</h2>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <button
            v-for="char in aiCharacters"
            :key="char.id"
            class="flex items-center gap-3 p-4 rounded-card border border-surface-subtle hover:border-primary hover:bg-primary/5 transition text-left"
            @click="selectAI(char)"
          >
            <div class="w-14 h-14 rounded-full overflow-hidden border-2 border-surface-subtle shrink-0">
              <img
                :src="char.avatar"
                :alt="char.name"
                class="w-full h-full object-cover"
                @error="($event.target as HTMLImageElement).src = '/uploads/avatar/default.jpg'"
              />
            </div>
            <div class="min-w-0">
              <div class="text-base font-medium text-ink">{{ char.name }}</div>
              <div class="text-xs text-primary mt-1" v-if="aiHistories[char.id]?.length">
                {{ aiHistories[char.id].length }} 条消息
              </div>
            </div>
          </button>
        </div>
        <div v-if="!aiCharacters.length" class="py-12 text-center text-sm text-ink-muted">
          加载中...
        </div>
      </div>

      <!-- 聊天 -->
      <div v-else class="bg-white rounded-card shadow-card overflow-hidden flex flex-col" style="height: calc(100vh - 140px); min-height: 500px;">
        <!-- 顶栏 -->
        <div class="flex items-center gap-3 px-4 h-14 border-b border-surface-subtle shrink-0">
          <button class="p-1 rounded hover:bg-surface-subtle transition" @click="backToAIList">
            <ArrowLeft :size="20" class="text-ink-secondary" />
          </button>
          <div class="w-9 h-9 rounded-full overflow-hidden border border-surface-subtle">
            <img
              :src="activeAI.avatar"
              :alt="activeAI.name"
              class="w-full h-full object-cover"
              @error="($event.target as HTMLImageElement).src = '/uploads/avatar/default.jpg'"
            />
          </div>
          <div class="flex-1 min-w-0">
            <div class="text-sm font-medium text-ink">{{ activeAI.name }}</div>
          </div>
          <button
            class="text-xs text-ink-muted hover:text-primary transition px-2 py-1 rounded hover:bg-surface-subtle"
            @click="clearAIHistory"
          >
            清空记录
          </button>
        </div>

        <!-- 消息列表 -->
        <div class="flex-1 overflow-y-auto px-4 py-4 space-y-4">
          <div v-if="!aiCurrentMessages().length" class="h-full flex items-center justify-center">
            <div class="text-center">
              <div class="w-16 h-16 rounded-full overflow-hidden mx-auto mb-3 border-2 border-surface-subtle">
                <img :src="activeAI.avatar" :alt="activeAI.name" class="w-full h-full object-cover" />
              </div>
              <p class="text-sm text-ink-muted">和 {{ activeAI.name }} 开始聊天吧！</p>
            </div>
          </div>

          <template v-for="(msg, idx) in aiCurrentMessages()" :key="idx">
            <!-- AI 消息 -->
            <div v-if="msg.role === 'assistant'" class="flex items-start gap-2">
              <div class="w-8 h-8 rounded-full overflow-hidden border border-surface-subtle shrink-0">
                <img :src="activeAI.avatar" :alt="activeAI.name" class="w-full h-full object-cover" />
              </div>
              <div class="max-w-[70%] bg-surface-subtle rounded-2xl rounded-tl-sm px-4 py-2">
                <p class="text-sm text-ink whitespace-pre-wrap break-words">{{ msg.content }}</p>
              </div>
            </div>

            <!-- 用户消息 -->
            <div v-else class="flex items-start gap-2 justify-end">
              <div class="max-w-[70%] bg-secondary rounded-2xl rounded-tr-sm px-4 py-2">
                <p class="text-sm text-white whitespace-pre-wrap break-words">{{ msg.content }}</p>
              </div>
              <div class="w-8 h-8 rounded-full overflow-hidden border border-surface-subtle shrink-0">
                <img
                  :src="userStore.userInfo?.avatar_url || '/uploads/avatar/default.jpg'"
                  alt="me"
                  class="w-full h-full object-cover"
                  @error="($event.target as HTMLImageElement).src = '/uploads/avatar/default.jpg'"
                />
              </div>
            </div>
          </template>

          <!-- 加载中提示 -->
          <div v-if="aiSending" class="flex items-start gap-2">
            <div class="w-8 h-8 rounded-full overflow-hidden border border-surface-subtle shrink-0">
              <img :src="activeAI.avatar" :alt="activeAI.name" class="w-full h-full object-cover" />
            </div>
            <div class="bg-surface-subtle rounded-2xl rounded-tl-sm px-4 py-3">
              <div class="flex gap-1">
                <span class="w-2 h-2 bg-ink-muted rounded-full animate-bounce" style="animation-delay: 0ms"></span>
                <span class="w-2 h-2 bg-ink-muted rounded-full animate-bounce" style="animation-delay: 150ms"></span>
                <span class="w-2 h-2 bg-ink-muted rounded-full animate-bounce" style="animation-delay: 300ms"></span>
              </div>
            </div>
          </div>

          <div ref="aiEnd"></div>
        </div>

        <!-- 输入区 -->
        <div class="border-t border-surface-subtle p-3 shrink-0">
          <div class="flex items-end gap-2">
            <textarea
              v-model="aiInput"
              :placeholder="`给${activeAI.name}发消息...`"
              rows="1"
              class="flex-1 resize-none bg-surface-subtle rounded-xl px-4 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-secondary/30 max-h-24"
              style="min-height: 38px;"
              :disabled="aiSending"
              @keydown="onAIKeydown"
            ></textarea>
            <button
              class="flex items-center justify-center w-10 h-10 rounded-xl bg-secondary text-white hover:bg-secondary-dark transition shrink-0 disabled:opacity-50"
              :disabled="!aiInput.trim() || aiSending"
              @click="sendAIMessage"
            >
              <Send :size="18" />
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
