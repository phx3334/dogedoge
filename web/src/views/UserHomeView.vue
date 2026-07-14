<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { UserPlus, UserCheck, Pencil, Video as VideoIcon, Rss, Heart, MessageCircle, Folder, Play, FileText } from 'lucide-vue-next'
import { getUserHome } from '@/api/user'
import { getUserVideos, deleteVideo } from '@/api/video'
import { getUserDynamics, getUserMixedDynamics, getDynamicFeed } from '@/api/dynamic'
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
    // 后端 UserHomeResp.is_followed 已返回"当前登录用户是否关注该用户"，
    // 用它驱动按钮灰态，避免重新加载 / 路由变动后"已关注"状态丢失。
    isFollowed.value = res.is_followed ?? false
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
    // 看自己的主页：走 feed 流，包含"自己 + 关注的人"的视频/文章/动态
    // 看他人主页：仅展示该用户自己的视频/文章/动态混合
    const res = isSelf.value
      ? await getDynamicFeed(page, 20)
      : await getUserMixedDynamics(userId.value, page, 20)
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

// 动态卡片跳转
function goUser(id: string) {
  if (id) router.push(`/user/${id}`)
}
function goVideo(id?: number) {
  if (id) router.push(`/video/${id}`)
}
function goArticle(id?: number) {
  if (id) router.push(`/article/${id}`)
}
function formatDuration(sec?: number) {
  if (!sec || sec <= 0) return ''
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return `${m}:${s.toString().padStart(2, '0')}`
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

      <!-- 动态 Tab（混合：视频 + 文章 + 图文动态） -->
      <div v-else-if="activeTab === 'dynamics'">
        <div v-if="dynamicLoading && !dynamics.length" class="text-center py-10 text-sm text-ink-muted">
          加载中...
        </div>
        <div v-else-if="dynamics.length" class="space-y-3">
          <article
            v-for="item in dynamics"
            :key="`${item.type}-${item.id}`"
            class="bg-white rounded-card shadow-card p-4"
          >
            <!-- 卡片头部 -->
            <header class="flex items-start gap-3 mb-3">
              <img
                :src="item.avatar_url || '/uploads/avatar/default.jpg'"
                :alt="item.username"
                class="w-12 h-12 rounded-full object-cover border border-surface-subtle shrink-0 cursor-pointer"
                @error="($event.target as HTMLImageElement).src = '/uploads/avatar/default.jpg'"
                @click="goUser(item.user_id)"
              />
              <div class="flex-1 min-w-0 pt-1">
                <div
                  class="text-[15px] font-semibold text-primary truncate cursor-pointer hover:text-primary-dark transition"
                  @click="goUser(item.user_id)"
                >
                  {{ item.username }}
                </div>
                <div class="flex items-center gap-1.5 mt-1 text-xs text-ink-muted">
                  <span>{{ formatDate(item.created_at) }}</span>
                  <span>·</span>
                  <span v-if="item.type === 'video'">投稿了视频</span>
                  <span v-else-if="item.type === 'article'">投稿了文章</span>
                  <span v-else>发布了动态</span>
                </div>
              </div>
              <span
                v-if="item.type === 'video'"
                class="flex items-center gap-1 px-2 py-0.5 bg-primary/10 text-primary text-xs rounded-full shrink-0"
              >
                <Play :size="11" /> 视频
              </span>
              <span
                v-else-if="item.type === 'article'"
                class="flex items-center gap-1 px-2 py-0.5 bg-secondary/10 text-secondary text-xs rounded-full shrink-0"
              >
                <FileText :size="11" /> 文章
              </span>
            </header>

            <!-- 视频类型 -->
            <template v-if="item.type === 'video'">
              <div class="block cursor-pointer group" @click="goVideo(item.video_id!)">
                <h3 class="text-[15px] font-semibold text-ink mb-2 group-hover:text-primary transition leading-snug">
                  {{ item.title }}
                </h3>
                <div class="relative aspect-video bg-surface-muted rounded-lg overflow-hidden">
                  <img
                    :src="item.cover_url"
                    :alt="item.title"
                    loading="lazy"
                    class="w-full h-full object-cover group-hover:scale-[1.02] transition-transform duration-300"
                  />
                  <div class="absolute inset-0 flex items-center justify-center opacity-0 group-hover:opacity-100 transition">
                    <div class="w-12 h-12 bg-black/60 rounded-full flex items-center justify-center">
                      <Play :size="24" class="text-white ml-1" fill="white" />
                    </div>
                  </div>
                  <div
                    v-if="item.duration"
                    class="absolute bottom-1.5 right-1.5 px-1.5 py-0.5 bg-black/70 text-white text-xs rounded"
                  >
                    {{ formatDuration(item.duration) }}
                  </div>
                </div>
              </div>
            </template>

            <!-- 文章类型 -->
            <template v-else-if="item.type === 'article'">
              <div class="block cursor-pointer group" @click="goArticle(item.article_id!)">
                <h3 class="text-[15px] font-semibold text-ink mb-2 group-hover:text-primary transition leading-snug">
                  {{ item.title }}
                </h3>
                <div v-if="item.cover_url" class="flex gap-3">
                  <div class="flex-1 min-w-0">
                    <p class="text-sm text-ink-secondary line-clamp-3 whitespace-pre-wrap leading-relaxed">
                      {{ item.content?.replace(/[#*`>-]/g, '').slice(0, 200) }}
                    </p>
                  </div>
                  <div class="w-32 h-20 rounded-lg overflow-hidden bg-surface-muted shrink-0">
                    <img :src="item.cover_url" :alt="item.title" loading="lazy" class="w-full h-full object-cover" />
                  </div>
                </div>
                <p v-else class="text-sm text-ink-secondary line-clamp-3 whitespace-pre-wrap leading-relaxed">
                  {{ item.content?.replace(/[#*`>-]/g, '').slice(0, 200) }}
                </p>
              </div>
            </template>

            <!-- 图文动态 -->
            <template v-else>
              <h3 v-if="item.title" class="text-[15px] font-semibold text-ink mb-1 leading-snug">{{ item.title }}</h3>
              <p class="text-sm text-ink leading-relaxed whitespace-pre-wrap break-words mb-3">{{ item.content }}</p>
              <div v-if="parseImages(item.images_json).length" class="mb-1">
                <div v-if="parseImages(item.images_json).length === 1" class="max-w-md">
                  <img :src="parseImages(item.images_json)[0]" alt="" class="w-full rounded-lg" />
                </div>
                <div v-else-if="parseImages(item.images_json).length <= 3" class="flex flex-wrap gap-2 max-w-md">
                  <img
                    v-for="(img, idx) in parseImages(item.images_json)"
                    :key="idx"
                    :src="img"
                    alt=""
                    class="w-30 h-30 object-cover rounded-lg"
                    style="width: 120px; height: 120px;"
                  />
                </div>
                <div v-else class="grid grid-cols-3 gap-2 max-w-sm">
                  <img
                    v-for="(img, idx) in parseImages(item.images_json)"
                    :key="idx"
                    :src="img"
                    alt=""
                    class="w-full aspect-square object-cover rounded-lg"
                  />
                </div>
              </div>
            </template>

            <!-- 底部操作栏 -->
            <div class="flex items-center gap-5 mt-3 pt-3 border-t border-surface-subtle text-sm text-ink-secondary">
              <span class="flex items-center gap-1.5">
                <Heart
                  :size="18"
                  :fill="item.is_liked ? 'currentColor' : 'none'"
                  :class="item.is_liked ? 'text-primary' : ''"
                />
                <span>{{ formatCount(item.like_count) }}</span>
              </span>
              <div class="flex items-center gap-1.5">
                <MessageCircle :size="18" />
                <span>{{ formatCount(item.comment_count) }}</span>
              </div>
              <span v-if="item.type === 'video'" class="text-xs text-ink-muted ml-auto">
                播放 {{ formatCount(item.play_count || 0) }}
              </span>
              <span v-else-if="item.type === 'article'" class="text-xs text-ink-muted ml-auto">
                阅读 {{ formatCount(item.view_count || 0) }}
              </span>
            </div>
          </article>
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
