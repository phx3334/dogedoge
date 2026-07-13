<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { Search, PenSquare, Bell, Star, History, User, LogOut, ChevronDown, Home, Rss } from 'lucide-vue-next'
import { useUserStore } from '@/stores/user'
import { useNotificationStore } from '@/stores/notification'
import { useMessageStore } from '@/stores/message'
import { calcLevel } from '@/utils/level'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const notifStore = useNotificationStore()
const messageStore = useMessageStore()

// 通知 + 私信 未读合计
const totalUnread = computed(() => notifStore.unreadCount + messageStore.unreadCount)

const searchKeyword = ref('')
const showUserMenu = ref(false)
const avatarError = ref(false)
const showSearchHistory = ref(false)
const searchHistory = ref<string[]>([])
let searchBlurTimer: ReturnType<typeof setTimeout> | null = null

const avatarUrl = computed(() => {
  if (avatarError.value) return '/uploads/avatar/default.jpg'
  return userStore.userInfo?.avatar_url || '/uploads/avatar/default.jpg'
})

const SEARCH_HISTORY_KEY = 'fake_doge_search_history'
const MAX_HISTORY = 10

function loadSearchHistory() {
  try {
    const raw = localStorage.getItem(SEARCH_HISTORY_KEY)
    searchHistory.value = raw ? JSON.parse(raw) : []
  } catch {
    searchHistory.value = []
  }
}

function saveSearchHistory(keyword: string) {
  const kw = keyword.trim()
  if (!kw) return
  loadSearchHistory()
  // 去重：移除已存在的相同关键词，再插入到头部
  const filtered = searchHistory.value.filter((k) => k !== kw)
  filtered.unshift(kw)
  // 最多保留 10 条
  searchHistory.value = filtered.slice(0, MAX_HISTORY)
  localStorage.setItem(SEARCH_HISTORY_KEY, JSON.stringify(searchHistory.value))
}

function onSearchFocus() {
  // 清除可能 pending 的 blur timer，避免重新 focus 时被关闭
  if (searchBlurTimer) {
    clearTimeout(searchBlurTimer)
    searchBlurTimer = null
  }
  loadSearchHistory()
  showSearchHistory.value = true
}

function onSearchBlur() {
  // 延迟关闭，允许点击历史项
  searchBlurTimer = setTimeout(() => {
    showSearchHistory.value = false
    searchBlurTimer = null
  }, 200)
}

function pickHistory(keyword: string) {
  searchKeyword.value = keyword
  showSearchHistory.value = false
  handleSearch()
}

function clearSearchHistory() {
  searchHistory.value = []
  localStorage.removeItem(SEARCH_HISTORY_KEY)
}

// 顶部分区导航（B站风格：推荐 + 22 个分区，分两行，每行 11 个）
const categoryTabs = [
  { key: '番剧', label: '番剧' },
  { key: '电影', label: '电影' },
  { key: '国创', label: '国创' },
  { key: '电视剧', label: '电视剧' },
  { key: '综艺', label: '综艺' },
  { key: '纪录片', label: '纪录片' },
  { key: '动画', label: '动画' },
  { key: '游戏', label: '游戏' },
  { key: '鬼畜', label: '鬼畜' },
  { key: '音乐', label: '音乐' },
  { key: '舞蹈', label: '舞蹈' },
  { key: '影视', label: '影视' },
  { key: '娱乐', label: '娱乐' },
  { key: '知识', label: '知识' },
  { key: '科技数码', label: '科技数码' },
  { key: '资讯', label: '资讯' },
  { key: '美食', label: '美食' },
  { key: '小剧场', label: '小剧场' },
  { key: '汽车', label: '汽车' },
  { key: '时尚美妆', label: '时尚美妆' },
  { key: '体育运动', label: '体育运动' },
  { key: '其他', label: '其他' },
]

const activeZone = computed(() => (route.name === 'home' ? (route.query.zone as string) || '' : ''))

function switchZone(zoneKey: string) {
  if (zoneKey === '') {
    router.push({ path: '/', query: {} })
  } else {
    router.push({ path: '/', query: { zone: zoneKey } })
  }
}

function handleSearch() {
  if (searchKeyword.value.trim()) {
    const kw = searchKeyword.value.trim()
    saveSearchHistory(kw)
    showSearchHistory.value = false
    router.push({ path: '/search', query: { keyword: kw } })
  }
}

function goLogin() {
  router.push('/login')
}

function goSpace() {
  showUserMenu.value = false
  const id = userStore.userInfo?.id
  if (id) router.push(`/user/${id}`)
  else router.push('/space/favorites')
}

async function handleLogout() {
  showUserMenu.value = false
  await userStore.logout()
  router.push('/')
}

function toggleUserMenu() {
  showUserMenu.value = !showUserMenu.value
}

function handleClickOutside(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (!target.closest('.user-menu-wrapper')) {
    showUserMenu.value = false
  }
}

onMounted(async () => {
  if (userStore.isLogin) {
    // 页面加载时从后端拉取最新用户信息，确保头像等数据正确
    await userStore.fetchUserInfo()
    notifStore.fetchUnreadCount()
    messageStore.fetchUnreadCount()
  }
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
/* 首页：导航覆盖在横幅图上，logo 与主导航文字改为白色 */
.home-header .flex.items-center > a span,
.home-header nav a {
  color: #fff !important;
}
.home-header nav a.border-primary {
  border-color: #fff !important;
}
.home-header nav a:not(.border-primary):hover {
  color: #fff !important;
}
</style>

<template>
  <header
    class="bili-header sticky top-0 z-50 min-[1440px]:max-w-none"
    :class="route.name === 'home' ? 'home-header bg-white' : 'bg-white border-b border-surface-subtle'"
  >
    <!-- 首页横幅图：作为顶栏背景，导航覆盖其上（无顶部留白） -->
    <div v-if="route.name === 'home'" class="relative">
      <img
        src="/images/Home_image.png"
        alt="首页横幅"
        class="w-full aspect-[6/1] object-cover [mask-image:linear-gradient(to_bottom,black_40%,transparent)]"
      />
      <!-- 导航浮于图片之上 -->
      <div class="absolute inset-x-0 top-0 h-14 flex items-center gap-4 px-4 max-w-[1280px] mx-auto min-[1440px]:max-w-none">
        <!-- Logo -->
        <router-link to="/" class="flex items-center gap-1 shrink-0">
          <span class="text-primary text-2xl font-bold tracking-tight">doge</span>
          <span class="text-[10px] leading-none mt-1 text-white">doge</span>
        </router-link>

        <!-- 主导航 -->
        <nav class="flex items-center gap-1 text-sm shrink-0">
          <router-link
            to="/"
            class="flex items-center gap-1 px-3 h-14 border-b-2 transition-colors"
            :class="route.name === 'home' ? 'border-primary text-primary' : 'border-transparent text-ink hover:text-primary'"
          >
            <Home :size="16" />
            <span>首页</span>
          </router-link>
          <router-link
            to="/dynamic"
            class="flex items-center gap-1 px-3 h-14 border-b-2 transition-colors"
            :class="String(route.name) === 'dynamic' ? 'border-primary text-primary' : 'border-transparent text-ink hover:text-primary'"
          >
            <Rss :size="16" />
            <span>动态</span>
          </router-link>
        </nav>

        <!-- 搜索框 -->
        <div class="flex-1 max-w-lg mx-auto relative">
          <div class="flex items-center bg-surface-subtle rounded-full overflow-hidden h-9 focus-within:ring-2 focus-within:ring-secondary/30 transition">
            <input
              v-model="searchKeyword"
              type="text"
              placeholder="搜索视频、文章"
              class="flex-1 h-full px-4 bg-transparent text-sm focus:outline-none"
              @keyup.enter="handleSearch"
              @focus="onSearchFocus"
              @blur="onSearchBlur"
            />
            <button
              class="h-full w-12 flex items-center justify-center bg-secondary text-white hover:bg-secondary-dark transition"
              @click="handleSearch"
            >
              <Search :size="18" />
            </button>
          </div>
          <!-- 搜索历史下拉 -->
          <div
            v-if="showSearchHistory && searchHistory.length"
            class="absolute top-full left-0 right-0 mt-1 bg-white rounded-lg shadow-lg border border-surface-subtle py-2 z-50 animate-fade-in"
          >
            <div class="flex items-center justify-between px-4 py-1">
              <span class="text-xs text-ink-muted">搜索历史</span>
              <button
                class="text-xs text-ink-muted hover:text-primary transition"
                @click="clearSearchHistory"
              >
                清空
              </button>
            </div>
            <button
              v-for="(kw, idx) in searchHistory"
              :key="idx"
              class="w-full text-left px-4 py-2 text-sm text-ink-secondary hover:bg-surface-subtle transition cursor-pointer"
              @mousedown.prevent="pickHistory(kw)"
            >
              {{ kw }}
            </button>
          </div>
        </div>

        <!-- 右侧操作 -->
        <div class="flex items-center gap-3 shrink-0">
          <!-- 投稿按钮 -->
          <router-link
            to="/upload"
            class="flex items-center gap-1 px-3 h-8 rounded border border-primary text-primary text-sm hover:bg-primary hover:text-white transition"
          >
            <PenSquare :size="16" />
            <span class="hidden sm:inline">投稿</span>
          </router-link>

          <template v-if="userStore.isLogin">
            <!-- 收藏 -->
            <router-link
              to="/space/favorites"
              class="hidden sm:flex items-center gap-1 text-ink-secondary hover:text-primary transition"
              title="收藏"
            >
              <Star :size="18" />
              <span class="text-sm">收藏</span>
            </router-link>
            <!-- 消息 -->
            <router-link
              to="/space/notifications"
              class="relative flex items-center gap-1 text-ink-secondary hover:text-primary transition"
              title="消息"
            >
              <Bell :size="18" />
              <span class="text-sm">消息</span>
              <span
                v-if="totalUnread > 0"
                class="absolute -top-1.5 -right-1.5 min-w-[16px] h-4 px-1 bg-primary text-white text-[10px] leading-4 rounded-full text-center"
              >
                {{ totalUnread > 99 ? '99+' : totalUnread }}
              </span>
            </router-link>
            <!-- 历史 -->
            <router-link
              to="/space/history"
              class="hidden sm:flex items-center gap-1 text-ink-secondary hover:text-primary transition"
              title="历史"
            >
              <History :size="18" />
              <span class="text-sm">历史</span>
            </router-link>
          </template>

          <!-- 头像 -->
          <div class="relative user-menu-wrapper">
            <button
              @click="userStore.isLogin ? toggleUserMenu() : goLogin()"
              class="flex items-center gap-1 h-8 rounded-full hover:bg-surface-subtle pr-1 transition"
              :class="{ 'pl-1': userStore.isLogin }"
            >
              <div class="w-7 h-7 rounded-full overflow-hidden border border-surface-muted">
                <img
                  :src="avatarUrl"
                  :alt="userStore.isLogin ? (userStore.userInfo?.username || 'avatar') : 'login'"
                  class="w-full h-full object-cover"
                  @error="avatarError = true"
                />
              </div>
              <ChevronDown v-if="userStore.isLogin" :size="14" class="text-ink-muted" />
            </button>

            <div
              v-if="userStore.isLogin && showUserMenu"
              class="absolute right-0 top-full mt-1 w-48 bg-white rounded-lg shadow-lg border border-surface-subtle py-1 z-50 animate-fade-in"
            >
              <div class="px-3 py-2 border-b border-surface-subtle">
                <div class="text-sm font-medium text-ink truncate">{{ userStore.userInfo?.username || '用户' }}</div>
                <div class="text-xs text-ink-muted mt-0.5">Lv{{ calcLevel(userStore.userInfo?.experience) }}</div>
              </div>
              <button
                class="w-full flex items-center gap-2 px-3 py-2 text-sm text-ink-secondary hover:bg-surface-subtle transition"
                @click="goSpace"
              >
                <User :size="16" />
                <span>个人中心</span>
              </button>
              <button
                class="w-full flex items-center gap-2 px-3 py-2 text-sm text-ink-secondary hover:bg-surface-subtle transition"
                @click="handleLogout"
              >
                <LogOut :size="16" />
                <span>退出登录</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- 分区导航子栏（22 个分区，分两行每行 11 个，仅首页，随图片一起吸顶） -->
      <div class="border-t border-surface-subtle bg-white">
        <div class="max-w-[1280px] mx-auto px-4 py-2 min-[1440px]:max-w-none">
          <div class="grid grid-cols-11 gap-x-2 gap-y-1">
            <button
              v-for="zone in categoryTabs"
              :key="zone.key"
              class="px-3 h-8 rounded-full text-sm whitespace-nowrap text-center transition-colors"
              :class="activeZone === zone.key ? 'bg-primary text-white font-medium' : 'text-ink-secondary hover:bg-surface-subtle hover:text-ink'"
              @click="switchZone(zone.key)"
            >
              {{ zone.label }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 非首页：正常 sticky 顶栏 -->
    <div v-else class="max-w-[1280px] mx-auto h-14 px-4 flex items-center gap-4 min-[1440px]:max-w-none">
      <!-- Logo -->
      <router-link to="/" class="flex items-center gap-1 shrink-0">
        <span class="text-primary text-2xl font-bold tracking-tight">doge</span>
        <span class="text-[10px] leading-none mt-1 text-white">doge</span>
      </router-link>

      <!-- 主导航 -->
      <nav class="flex items-center gap-1 text-sm shrink-0">
        <router-link
          to="/"
          class="flex items-center gap-1 px-3 h-14 border-b-2 transition-colors"
          :class="route.name === 'home' ? 'border-primary text-primary' : 'border-transparent text-ink hover:text-primary'"
        >
          <Home :size="16" />
          <span>首页</span>
        </router-link>
        <router-link
          to="/dynamic"
          class="flex items-center gap-1 px-3 h-14 border-b-2 transition-colors"
          :class="route.name === 'dynamic' ? 'border-primary text-primary' : 'border-transparent text-ink hover:text-primary'"
        >
          <Rss :size="16" />
          <span>动态</span>
        </router-link>
      </nav>

      <!-- 搜索框 (B站风格：圆角灰色背景+蓝色搜索按钮) -->
      <div class="flex-1 max-w-lg mx-auto relative">
        <div class="flex items-center bg-surface-subtle rounded-full overflow-hidden h-9 focus-within:ring-2 focus-within:ring-secondary/30 transition">
          <input
            v-model="searchKeyword"
            type="text"
            placeholder="搜索视频、文章"
            class="flex-1 h-full px-4 bg-transparent text-sm focus:outline-none"
            @keyup.enter="handleSearch"
            @focus="onSearchFocus"
            @blur="onSearchBlur"
          />
          <button
            class="h-full w-12 flex items-center justify-center bg-secondary text-white hover:bg-secondary-dark transition"
            @click="handleSearch"
          >
            <Search :size="18" />
          </button>
        </div>
        <!-- 搜索历史下拉 -->
        <div
          v-if="showSearchHistory && searchHistory.length"
          class="absolute top-full left-0 right-0 mt-1 bg-white rounded-lg shadow-lg border border-surface-subtle py-2 z-50 animate-fade-in"
        >
          <div class="flex items-center justify-between px-4 py-1">
            <span class="text-xs text-ink-muted">搜索历史</span>
            <button
              class="text-xs text-ink-muted hover:text-primary transition"
              @click="clearSearchHistory"
            >
              清空
            </button>
          </div>
          <button
            v-for="(kw, idx) in searchHistory"
            :key="idx"
            class="w-full text-left px-4 py-2 text-sm text-ink-secondary hover:bg-surface-subtle transition cursor-pointer"
            @mousedown.prevent="pickHistory(kw)"
          >
            {{ kw }}
          </button>
        </div>
      </div>

      <!-- 右侧操作 -->
      <div class="flex items-center gap-3 shrink-0">
        <!-- 投稿按钮 -->
        <router-link
          to="/upload"
          class="flex items-center gap-1 px-3 h-8 rounded border border-primary text-primary text-sm hover:bg-primary hover:text-white transition"
        >
          <PenSquare :size="16" />
          <span class="hidden sm:inline">投稿</span>
        </router-link>

        <template v-if="userStore.isLogin">
          <!-- 收藏 -->
          <router-link
            to="/space/favorites"
            class="hidden sm:flex items-center gap-1 text-ink-secondary hover:text-primary transition"
            title="收藏"
          >
            <Star :size="18" />
            <span class="text-sm">收藏</span>
          </router-link>
          <!-- 消息 -->
          <router-link
            to="/space/notifications"
            class="relative flex items-center gap-1 text-ink-secondary hover:text-primary transition"
            title="消息"
          >
            <Bell :size="18" />
            <span class="text-sm">消息</span>
            <span
              v-if="totalUnread > 0"
              class="absolute -top-1.5 -right-1.5 min-w-[16px] h-4 px-1 bg-primary text-white text-[10px] leading-4 rounded-full text-center"
            >
              {{ totalUnread > 99 ? '99+' : totalUnread }}
            </span>
          </router-link>
          <!-- 历史 -->
          <router-link
            to="/space/history"
            class="hidden sm:flex items-center gap-1 text-ink-secondary hover:text-primary transition"
            title="历史"
          >
            <History :size="18" />
            <span class="text-sm">历史</span>
          </router-link>
        </template>

        <!-- 头像：登录显示用户头像并展开菜单；未登录显示默认头像并跳转登录 -->
        <div class="relative user-menu-wrapper">
          <button
            @click="userStore.isLogin ? toggleUserMenu() : goLogin()"
            class="flex items-center gap-1 h-8 rounded-full hover:bg-surface-subtle pr-1 transition"
            :class="{ 'pl-1': userStore.isLogin }"
          >
            <div class="w-7 h-7 rounded-full overflow-hidden border border-surface-muted">
              <img
                :src="avatarUrl"
                :alt="userStore.isLogin ? (userStore.userInfo?.username || 'avatar') : 'login'"
                class="w-full h-full object-cover"
                @error="avatarError = true"
              />
            </div>
            <ChevronDown v-if="userStore.isLogin" :size="14" class="text-ink-muted" />
          </button>

          <!-- 已登录时显示下拉菜单 -->
          <div
            v-if="userStore.isLogin && showUserMenu"
            class="absolute right-0 top-full mt-1 w-48 bg-white rounded-lg shadow-lg border border-surface-subtle py-1 z-50 animate-fade-in"
          >
            <div class="px-3 py-2 border-b border-surface-subtle">
              <div class="text-sm font-medium text-ink truncate">{{ userStore.userInfo?.username || '用户' }}</div>
              <div class="text-xs text-ink-muted mt-0.5">Lv{{ calcLevel(userStore.userInfo?.experience) }}</div>
            </div>
            <button
              class="w-full flex items-center gap-2 px-3 py-2 text-sm text-ink-secondary hover:bg-surface-subtle transition"
              @click="goSpace"
            >
              <User :size="16" />
              <span>个人中心</span>
            </button>
            <button
              class="w-full flex items-center gap-2 px-3 py-2 text-sm text-ink-secondary hover:bg-surface-subtle transition"
              @click="handleLogout"
            >
              <LogOut :size="16" />
              <span>退出登录</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  </header>
</template>
