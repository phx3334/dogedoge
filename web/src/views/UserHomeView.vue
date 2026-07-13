<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { UserPlus, UserCheck, Pencil, Video as VideoIcon, Rss, Heart, MessageCircle, Folder } from 'lucide-vue-next'
import { getUserHome } from '@/api/user'
import { getUserVideos, deleteVideo } from '@/api/video'
import { getUserDynamics } from '@/api/dynamic'
import { followUser, unfollowUser } from '@/api/interaction'
import { useUserStore } from '@/stores/user'
import type { HomeVideoInfo, DynamicItem, UserHomeResp, FavoriteFolderInfo } from '@/types'
import VideoCard from '@/components/common/VideoCard.vue'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const userId = computed(() => route.params.id as string)
const activeTab = ref<'videos' | 'dynamics' | 'favorites'>('videos')

// 用户主页数据
const homeData = ref<UserHomeResp | null>(null)
const isFollowed = ref(false)
const followLoading = ref(false)
const homeLoading = ref(false)

// 视频列表
const videos = ref<HomeVideoInfo[]>([])
const videoPage = ref(1)
const videoTotal = ref(0)
const videoLoading = ref(false)

// 动态列表
const dynamics = ref<DynamicItem[]>([])
const dynamicPage = ref(1)
const dynamicTotal = ref(0)
const dynamicLoading = ref(false)

// 收藏夹
const favoriteFolders = ref<FavoriteFolderInfo[]>([])

const isSelf = computed(() => userStore.userInfo?.id === userId.value)

import { calcLevel } from '@/utils/level'

const level = computed(() => {
  if (!homeData.value) return 1
  return calcLevel(homeData.value.experience)
})

async function loadHomeData() {
  homeLoading.value = true
  try {
    const res = await getUserHome(userId.value, 1, 20)
    homeData.value = res
    isFollowed.value = false // 后端 UserHomeResp 未返回 is_followed，需要单独查询
    videos.value = res.videos || []
    videoTotal.value = res.video_count || 0
    favoriteFolders.value = res.favorite_folders || []
  } catch {
    homeData.value = null
  } finally {
    homeLoading.value = false
  }
}

async function loadVideos(page: number = 1) {
  videoLoading.value = true
  try {
    const res = await getUserVideos(userId.value, page, 20)
    videos.value = res.list || []
    videoTotal.value = res.total || 0
    videoPage.value = page
  } finally {
    videoLoading.value = false
  }
}

// 删除自己投稿的视频（仅 isSelf 时展示删除按钮）
const deletingId = ref<number | null>(null)
async function onDeleteVideo(video: HomeVideoInfo) {
  if (!window.confirm(`确定删除视频《${video.title || '未命名'}》吗？此操作不可恢复。`)) return
  deletingId.value = video.id
  try {
    await deleteVideo(video.id)
    videos.value = videos.value.filter((v) => v.id !== video.id)
    videoTotal.value = Math.max(0, videoTotal.value - 1)
  } catch {
    // 错误提示由请求拦截器统一处理
  } finally {
    deletingId.value = null
  }
}

async function loadDynamics(page: number = 1) {
  dynamicLoading.value = true
  try {
    const res = await getUserDynamics(userId.value, page, 20)
    dynamics.value = res.list || []
    dynamicTotal.value = res.total || 0
    dynamicPage.value = page
  } finally {
    dynamicLoading.value = false
  }
}

function switchTab(tab: 'videos' | 'dynamics' | 'favorites') {
  if (activeTab.value === tab) return
  activeTab.value = tab
  if (tab === 'dynamics' && !dynamics.value.length) loadDynamics(1)
}

async function toggleFollow() {
  if (!userStore.isLogin) {
    router.push('/login')
    return
  }
  if (followLoading.value) return
  followLoading.value = true
  try {
    if (isFollowed.value) {
      await unfollowUser(userId.value)
      isFollowed.value = false
    } else {
      await followUser(userId.value)
      isFollowed.value = true
    }
  } finally {
    followLoading.value = false
  }
}

function formatCount(n: number) {
  if (n >= 10000) return (n / 10000).toFixed(1) + '万'
  return String(n)
}

function formatDate(s: string) {
  return new Date(s).toLocaleString('zh-CN')
}

function parseImages(json: string): string[] {
  if (!json) return []
  try {
    return JSON.parse(json)
  } catch {
    return []
  }
}

function goFolder(id: number) {
  router.push(`/space/favorites?folder=${id}`)
}

function goMessage() {
  router.push(`/space/notifications?tab=message&peer=${userId.value}`)
}

watch(
  () => route.params.id,
  (id) => {
    if (!id) return
    homeData.value = null
    videos.value = []
    dynamics.value = []
    favoriteFolders.value = []
    activeTab.value = 'videos'
    loadHomeData()
  },
  { immediate: true }
)
</script>

<template>
  <div class="bili-user-home">
    <!-- 顶部横幅 + 用户信息（B站风格） -->
    <div class="user-banner">
      <!-- 背景横幅 -->
      <div class="h-40 bg-gradient-to-r from-primary/20 via-secondary/20 to-primary/20 relative overflow-hidden">
        <div class="absolute inset-0 bg-gradient-to-b from-transparent to-white/50" />
      </div>

      <!-- 用户信息卡 -->
      <div class="max-w-[1280px] mx-auto min-[1440px]:max-w-none px-4 -mt-12 relative">
        <div class="bg-white rounded-card shadow-card p-6">
          <div class="flex items-start gap-4">
            <!-- 头像 -->
            <div class="w-20 h-20 rounded-full overflow-hidden border-4 border-white shadow-lg shrink-0 bg-surface-muted">
              <img
                :src="homeData?.avatar_url || '/uploads/avatar/default.jpg'"
                :alt="homeData?.username"
                class="w-full h-full object-cover"
                @error="($event.target as HTMLImageElement).src = '/uploads/avatar/default.jpg'"
              />
            </div>

            <!-- 用户名 + 签名 -->
            <div class="flex-1 min-w-0 pt-2">
              <div class="flex items-center gap-2">
                <h1 class="text-xl font-bold text-ink">{{ homeData?.username || '加载中...' }}</h1>
                <span class="px-2 py-0.5 rounded text-xs font-medium text-white"
                  :class="`bg-gradient-to-r from-secondary to-primary`"
                >
                  Lv{{ level }}
                </span>
              </div>
              <p class="text-sm text-ink-muted mt-2">{{ homeData?.signature || '这个人很懒，什么都没留下~' }}</p>
            </div>

            <!-- 操作按钮 -->
            <div class="shrink-0 pt-2">
              <button
                v-if="!isSelf"
                class="flex items-center gap-1 px-5 h-9 rounded-full text-sm font-medium transition disabled:opacity-60"
                :class="isFollowed
                  ? 'bg-surface-subtle text-ink-secondary hover:bg-surface-muted'
                  : 'bg-primary text-white hover:bg-primary-dark'"
                :disabled="followLoading"
                @click="toggleFollow"
              >
                <component :is="isFollowed ? UserCheck : UserPlus" :size="16" />
                {{ isFollowed ? '已关注' : '关注' }}
              </button>
              <button
                v-if="userStore.isLogin"
                class="flex items-center gap-1 px-5 h-9 rounded-full text-sm font-medium bg-surface-subtle text-ink-secondary hover:bg-surface-muted transition"
                @click="goMessage"
              >
                <MessageCircle :size="16" />
                私信
              </button>
              <button
                v-else
                class="flex items-center gap-1 px-5 h-9 rounded-full text-sm font-medium bg-surface-subtle text-ink-secondary hover:bg-surface-muted transition"
                @click="router.push('/space/profile')"
              >
                <Pencil :size="16" />
                编辑资料
              </button>
            </div>
          </div>

          <!-- 数据统计 -->
          <div class="mt-4 flex items-center gap-6 border-t border-surface-subtle pt-4">
            <div class="text-center cursor-pointer hover:text-primary transition" @click="router.push(`/space/following?user_id=${userId}`)">
              <div class="text-lg font-bold text-ink">{{ formatCount(homeData?.following_count || 0) }}</div>
              <div class="text-xs text-ink-muted">关注</div>
            </div>
            <div class="text-center cursor-pointer hover:text-primary transition" @click="router.push(`/space/followers?user_id=${userId}`)">
              <div class="text-lg font-bold text-ink">{{ formatCount(homeData?.fans_count || 0) }}</div>
              <div class="text-xs text-ink-muted">粉丝</div>
            </div>
            <div class="text-center">
              <div class="text-lg font-bold text-ink">{{ formatCount(homeData?.total_likes_received || 0) }}</div>
              <div class="text-xs text-ink-muted">获赞</div>
            </div>
            <div class="text-center">
              <div class="text-lg font-bold text-ink">{{ formatCount(homeData?.total_play_count || 0) }}</div>
              <div class="text-xs text-ink-muted">播放</div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Tab 切换 + 内容区 -->
    <div class="max-w-[1280px] mx-auto min-[1440px]:max-w-none px-4 mt-4">
      <!-- Tab 栏 -->
      <div class="flex items-center gap-6 mb-4 border-b border-surface-muted">
        <button
          class="relative flex items-center gap-1.5 pb-3 text-sm transition"
          :class="activeTab === 'videos' ? 'text-primary font-medium' : 'text-ink-secondary hover:text-ink'"
          @click="switchTab('videos')"
        >
          <VideoIcon :size="16" />
          投稿
          <span class="text-xs text-ink-muted ml-1">{{ homeData?.video_count || 0 }}</span>
          <span
            v-if="activeTab === 'videos'"
            class="absolute bottom-0 left-0 right-0 h-0.5 bg-primary rounded-full"
          />
        </button>
        <button
          class="relative flex items-center gap-1.5 pb-3 text-sm transition"
          :class="activeTab === 'dynamics' ? 'text-primary font-medium' : 'text-ink-secondary hover:text-ink'"
          @click="switchTab('dynamics')"
        >
          <Rss :size="16" />
          动态
          <span
            v-if="activeTab === 'dynamics'"
            class="absolute bottom-0 left-0 right-0 h-0.5 bg-primary rounded-full"
          />
        </button>
        <button
          v-if="isSelf"
          class="relative flex items-center gap-1.5 pb-3 text-sm transition"
          :class="activeTab === 'favorites' ? 'text-primary font-medium' : 'text-ink-secondary hover:text-ink'"
          @click="switchTab('favorites')"
        >
          <Folder :size="16" />
          收藏
          <span
            v-if="activeTab === 'favorites'"
            class="absolute bottom-0 left-0 right-0 h-0.5 bg-primary rounded-full"
          />
        </button>
      </div>

      <!-- 视频 Tab -->
      <div v-if="activeTab === 'videos'">
        <div v-if="videoLoading && !videos.length" class="text-center py-10 text-sm text-ink-muted">
          加载中...
        </div>
        <div
          v-else-if="videos.length"
          class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4"
        >
          <div v-for="video in videos" :key="video.id" class="relative group">
            <VideoCard :video="video" />
            <button
              v-if="isSelf"
              class="absolute top-2 right-2 z-10 flex items-center gap-1 px-2 h-7 rounded-full text-xs font-medium bg-black/60 text-white opacity-0 group-hover:opacity-100 hover:bg-red-500 transition disabled:opacity-40"
              :disabled="deletingId === video.id"
              @click.stop="onDeleteVideo(video)"
            >
              {{ deletingId === video.id ? '删除中' : '删除' }}
            </button>
          </div>
        </div>
        <EmptyState v-else text="该用户暂无投稿" />
        <Pagination
          v-if="videos.length"
          :current="videoPage"
          :total="videoTotal"
          :page-size="20"
          @change="(p) => loadVideos(p)"
        />
      </div>

      <!-- 动态 Tab -->
      <div v-else-if="activeTab === 'dynamics'">
        <div v-if="dynamicLoading && !dynamics.length" class="text-center py-10 text-sm text-ink-muted">
          加载中...
        </div>
        <div v-else-if="dynamics.length" class="space-y-4">
          <div
            v-for="item in dynamics"
            :key="item.id"
            class="bg-white rounded-card shadow-card p-4"
          >
            <div class="flex items-center gap-2 mb-2">
              <img
                :src="item.avatar_url || '/uploads/avatar/default.jpg'"
                :alt="item.username"
                class="w-8 h-8 rounded-full object-cover"
                @error="($event.target as HTMLImageElement).src = '/uploads/avatar/default.jpg'"
              />
              <div class="flex-1 min-w-0">
                <div class="text-sm font-medium text-ink">{{ item.username }}</div>
                <div class="text-xs text-ink-muted">{{ formatDate(item.created_at) }}</div>
              </div>
            </div>
            <h3 v-if="item.title" class="text-sm font-medium text-ink mb-1">{{ item.title }}</h3>
            <p class="text-sm text-ink-secondary whitespace-pre-wrap break-words">{{ item.content }}</p>
            <!-- 图片网格 -->
            <div
              v-if="parseImages(item.images_json).length"
              class="grid grid-cols-3 gap-1 mt-3"
            >
              <div
                v-for="(img, idx) in parseImages(item.images_json).slice(0, 9)"
                :key="idx"
                class="aspect-square rounded overflow-hidden bg-surface-muted"
              >
                <img :src="img" :alt="`图片${idx + 1}`" loading="lazy" class="w-full h-full object-cover" />
              </div>
            </div>
            <div class="flex items-center gap-5 mt-3 text-xs text-ink-muted">
              <span class="flex items-center gap-1">
                <Heart :size="14" :fill="item.is_liked ? 'currentColor' : 'none'" :class="item.is_liked ? 'text-primary' : ''" />
                {{ formatCount(item.like_count) }}
              </span>
              <span class="flex items-center gap-1">
                <MessageCircle :size="14" />
                {{ formatCount(item.comment_count) }}
              </span>
            </div>
          </div>
        </div>
        <EmptyState v-else text="该用户暂无动态" />
        <Pagination
          v-if="dynamics.length"
          :current="dynamicPage"
          :total="dynamicTotal"
          :page-size="20"
          @change="(p) => loadDynamics(p)"
        />
      </div>

      <!-- 收藏 Tab (仅自己可见) -->
      <div v-else-if="activeTab === 'favorites'">
        <div v-if="favoriteFolders.length" class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
          <div
            v-for="folder in favoriteFolders"
            :key="folder.id"
            class="bg-white rounded-card shadow-card p-4 cursor-pointer hover:shadow-card-hover transition"
            @click="goFolder(folder.id)"
          >
            <div class="aspect-video bg-surface-subtle rounded mb-3 flex items-center justify-center">
              <Folder :size="40" class="text-ink-muted" />
            </div>
            <h3 class="text-sm font-medium text-ink truncate">{{ folder.title }}</h3>
            <p class="text-xs text-ink-muted mt-1">{{ folder.is_default ? '默认收藏夹' : '自定义收藏夹' }}</p>
          </div>
        </div>
        <EmptyState v-else text="暂无收藏夹" />
      </div>
    </div>
  </div>
</template>
