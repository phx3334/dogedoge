<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Heart, Coins, Star, Share2, Bell, X, Play, MessageSquare } from 'lucide-vue-next'
import { getVideoDetail, getHomeVideoList } from '@/api/video'
import {
  getDanmaku,
  likeVideo,
  unlikeVideo,
  favoriteVideo,
  unfavoriteVideo,
  followUser,
  unfollowUser,
} from '@/api/interaction'
import { recordVideoView } from '@/api/history'
import { triggerDailyWatchOnce } from '@/api/daily'
import { coinVideo } from '@/api/coin'
import { getFavoriteFolders } from '@/api/favorite'
import { useUserStore } from '@/stores/user'
import type { VideoDetailResp, DanmakuItem, HomeVideoInfo } from '@/types'
import VideoPlayer from '@/components/video/VideoPlayer.vue'
import CommentTree from '@/components/comment/CommentTree.vue'
import VideoCard from '@/components/common/VideoCard.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { useCommentScroll } from '@/composables/useCommentScroll'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

// 通知点击跳转后滚动并高亮对应评论
useCommentScroll()

const videoId = computed(() => Number(route.params.id))
const video = ref<VideoDetailResp | null>(null)
const danmakuList = ref<DanmakuItem[]>([])
const recommendList = ref<HomeVideoInfo[]>([])
const loading = ref(true)

// 从历史记录跳转时携带的播放进度（秒），传给 VideoPlayer 自动定位
const resumeTime = computed(() => {
  const t = route.query.t
  if (!t) return 0
  const n = Number(t)
  return n > 0 && isFinite(n) ? n : 0
})

// 交互状态
const isLiked = ref(false)
const isFavorited = ref(false)
const isFollowed = ref(false)

// 弹窗
const showCoinModal = ref(false)
const coinAmount = ref<1 | 2>(1)
const defaultFolderId = ref(0)

// 播放器引用
const playerRef = ref<InstanceType<typeof VideoPlayer> | null>(null)

const playCountText = computed(() => {
  const n = video.value?.play_count || 0
  return n >= 10000 ? (n / 10000).toFixed(1) + '万' : String(n)
})

function showToast(msg: string) {
  const toast = document.createElement('div')
  toast.textContent = msg
  toast.style.cssText =
    'position:fixed;top:60px;left:50%;transform:translateX(-50%);background:#333;color:#fff;padding:8px 16px;border-radius:6px;z-index:9999;font-size:14px;box-shadow:0 2px 8px rgba(0,0,0,0.15)'
  document.body.appendChild(toast)
  setTimeout(() => toast.remove(), 2500)
}

function requireLogin(): boolean {
  if (!userStore.isLogin) {
    showToast('请先登录')
    router.push('/login')
    return false
  }
  return true
}

async function loadVideoDetail() {
  loading.value = true
  try {
    const res = await getVideoDetail(videoId.value)
    video.value = res
    isLiked.value = res.interaction.is_liked
    isFavorited.value = res.interaction.is_favorited
    isFollowed.value = res.interaction.is_followed
  } catch {
    video.value = null
  } finally {
    loading.value = false
  }
}

async function loadDanmaku() {
  try {
    const list = await getDanmaku(videoId.value)
    danmakuList.value = list
    // 同步弹幕总数：用真实加载到的弹幕条数覆盖详情页缓存中的计数字段，
    // 避免 Redis/MySQL 计数不一致导致前端显示错误
    if (video.value && Array.isArray(list)) {
      video.value.danmaku_count = list.length
    }
    // 直接调用 VideoPlayer 暴露的方法加载弹幕到播放器
    // 不仅依赖 watch 触发，确保弹幕能正确加载
    if (list && list.length > 0) {
      playerRef.value?.loadDanmakuList(list)
    }
  } catch {
    danmakuList.value = []
  }
}

async function loadRecommend() {
  try {
    const res = await getHomeVideoList('', 6)
    recommendList.value = (res.list || []).filter((v) => v.id !== videoId.value).slice(0, 5)
  } catch {
    recommendList.value = []
  }
}

async function loadDefaultFolder() {
  if (!userStore.isLogin) return
  try {
    const folders = await getFavoriteFolders()
    const def = folders.find((f) => f.is_default) || folders[0]
    if (def) defaultFolderId.value = def.id
  } catch {
    // ignore
  }
}

async function toggleLike() {
  if (!requireLogin() || !video.value) return
  const wasLiked = isLiked.value
  isLiked.value = !wasLiked
  video.value.likes_count += wasLiked ? -1 : 1
  try {
    if (wasLiked) {
      await unlikeVideo(videoId.value)
    } else {
      await likeVideo(videoId.value)
    }
  } catch {
    isLiked.value = wasLiked
    video.value.likes_count += wasLiked ? 1 : -1
  }
}

async function doCoin() {
  if (!requireLogin() || !video.value) return
  try {
    const res = await coinVideo(videoId.value, coinAmount.value)
    if (res.added === 0) {
      showToast('您已对该视频投过2个硬币')
      showCoinModal.value = false
      return
    }
    video.value.coin_count += res.added
    showCoinModal.value = false
    showToast(`投币 ${res.added} 枚成功`)
  } catch {
    // 错误已由拦截器提示
  }
}

async function toggleFavorite() {
  if (!requireLogin() || !video.value) return
  const wasFav = isFavorited.value
  isFavorited.value = !wasFav
  // 乐观更新：收藏 +1，取消收藏 -1 但不低于 0
  if (wasFav) {
    video.value.fav_count = Math.max(0, video.value.fav_count - 1)
  } else {
    video.value.fav_count += 1
  }
  try {
    if (wasFav) {
      await unfavoriteVideo(videoId.value, defaultFolderId.value)
      showToast('已取消收藏')
    } else {
      await favoriteVideo(videoId.value, defaultFolderId.value)
      showToast('收藏成功')
    }
  } catch {
    // 回滚乐观更新
    isFavorited.value = wasFav
    if (wasFav) {
      video.value.fav_count += 1
    } else {
      video.value.fav_count = Math.max(0, video.value.fav_count - 1)
    }
  }
}

async function toggleFollow() {
  if (!requireLogin() || !video.value) return
  const wasFollowed = isFollowed.value
  isFollowed.value = !wasFollowed
  // 乐观更新粉丝数
  video.value.author.fans_count += wasFollowed ? -1 : 1
  try {
    if (wasFollowed) {
      await unfollowUser(video.value.author.id)
      showToast('已取消关注')
    } else {
      await followUser(video.value.author.id)
      showToast('关注成功')
    }
  } catch {
    isFollowed.value = wasFollowed
    video.value.author.fans_count += wasFollowed ? 1 : -1
  }
}

// 弹幕发送成功后，仅更新弹幕计数 +1
// 不重新加载弹幕列表：reloadDanmakuList 会清空播放器弹幕队列并重新加载，
// 导致刚通过 plugin.emit() 即时显示的弹幕被清除，用户感觉弹幕"消失"
// 下次刷新页面时会从后端加载完整列表（含新弹幕），数据最终一致
function onDanmakuSent() {
  if (video.value) {
    video.value.danmaku_count += 1
  }
}

// 记录当前视频的观看历史
// 在路由切换和组件卸载时调用，确保观看进度被正确保存
// 使用视频ID参数而非 videoId 计算属性，因为路由切换时计算属性已指向新视频
function recordCurrentHistory(vid: number, duration: number) {
  const watched = playerRef.value?.getCurrentTime() || 0
  if (vid > 0 && watched > 0) {
    // fire-and-forget 但用 catch 兜底，避免未处理的 Promise 拒绝
    recordVideoView(vid, watched, duration || 0).catch(() => {})
  }
}

// 定时记录观看进度：每 15 秒自动上报一次，确保历史记录实时更新
// 解决用户观看中途关闭浏览器/切标签页导致历史不记录的问题
let historyTimer: ReturnType<typeof setInterval> | null = null

function startHistoryTimer() {
  stopHistoryTimer()
  historyTimer = setInterval(() => {
    if (video.value && videoId.value > 0) {
      recordCurrentHistory(videoId.value, video.value.duration || 0)
    }
  }, 15000) // 15 秒上报一次
}

function stopHistoryTimer() {
  if (historyTimer) {
    clearInterval(historyTimer)
    historyTimer = null
  }
}

async function initPage() {
  // 先加载视频详情，确保 video.value 存在后再加载弹幕并同步计数
  await loadVideoDetail()
  await loadDanmaku()
  loadRecommend()
  loadDefaultFolder()
  // 视频加载完成后启动定时记录
  startHistoryTimer()
  // 触发每日观看任务（后端按天幂等）
  if (userStore.isLogin) triggerDailyWatchOnce()
}

onMounted(() => {
  initPage()
})

// 路由参数变化时重新加载
watch(() => route.params.id, async (newId, oldId) => {
  if (newId) {
    // 先停止旧视频的定时记录
    stopHistoryTimer()
    // 先记录上一个视频的观看历史（在状态重置前捕获）
    if (oldId) {
      const oldVid = Number(oldId)
      const oldDuration = video.value?.duration || 0
      recordCurrentHistory(oldVid, oldDuration)
    }
    video.value = null
    danmakuList.value = []
    loading.value = true
    // 重置交互状态，避免新视频继承旧视频的点赞/收藏/关注/投币弹窗
    isLiked.value = false
    isFavorited.value = false
    isFollowed.value = false
    showCoinModal.value = false
    coinAmount.value = 1
    await loadVideoDetail()
    await loadDanmaku()
    loadRecommend()
    // 新视频加载完成，重新启动定时记录
    startHistoryTimer()
  }
})

// 在子组件销毁前获取播放进度
onBeforeUnmount(() => {
  stopHistoryTimer()
  recordCurrentHistory(videoId.value, video.value?.duration || 0)
})
</script>

<template>
  <div class="max-w-[1280px] mx-auto min-[1440px]:max-w-none px-4 py-4">
    <!-- 骨架屏 -->
    <div v-if="loading" class="flex gap-4">
      <div class="flex-1">
        <div class="w-full aspect-video bg-surface-muted animate-pulse rounded" />
        <div class="h-7 bg-surface-muted rounded animate-pulse mt-4 w-3/4" />
        <div class="h-12 bg-surface-muted rounded animate-pulse mt-3" />
      </div>
      <div class="w-[320px] space-y-3">
        <div v-for="i in 5" :key="i" class="h-20 bg-surface-muted rounded animate-pulse" />
      </div>
    </div>

    <div v-else-if="video" class="flex gap-4">
      <!-- 左侧主区域 -->
      <div class="flex-1 min-w-0">
        <!-- 标题（B站风格：标题在播放器上方） -->
        <h1 class="mb-3 text-xl font-medium text-ink leading-snug">{{ video.title }}</h1>

        <!-- 播放器 -->
        <VideoPlayer
          ref="playerRef"
          :url="video.play_url"
          :cover-url="video.cover_url"
          :danmaku-list="danmakuList"
          :video-id="videoId"
          :resume-time="resumeTime"
          @danmaku-sent="onDanmakuSent"
        />

        <!-- 数据统计栏 -->
        <div class="mt-3 flex items-center gap-4 text-xs text-ink-muted px-1">
          <span class="flex items-center gap-1">
            <Play :size="14" />
            {{ playCountText }} 播放
          </span>
          <span class="flex items-center gap-1">
            <Heart :size="14" />
            {{ video.likes_count }} 点赞
          </span>
          <span class="flex items-center gap-1">
            <Coins :size="14" />
            {{ video.coin_count }} 投币
          </span>
          <span class="flex items-center gap-1">
            <Star :size="14" />
            {{ video.fav_count }} 收藏
          </span>
          <span class="flex items-center gap-1">
            <MessageSquare :size="14" />
            {{ video.danmaku_count }} 弹幕
          </span>
          <span v-if="video.zone" class="ml-auto px-2 py-0.5 bg-surface-subtle rounded text-ink-secondary">
            {{ video.zone }}
          </span>
        </div>

        <!-- UP主信息卡 + 互动栏（B站风格：合并为一行） -->
        <div class="mt-3 bg-white rounded-card p-4">
          <div class="flex items-center gap-3">
            <!-- UP主头像 -->
            <img
              :src="video.author.avatar_url || '/uploads/avatar/default.jpg'"
              :alt="video.author.username"
              class="w-12 h-12 rounded-full object-cover bg-surface-muted shrink-0 cursor-pointer border-2 border-surface-muted"
              @error="($event.target as HTMLImageElement).src = '/uploads/avatar/default.jpg'"
              @click="router.push(`/user/${video.author.id}`)"
            />
            <div class="flex-1 min-w-0 cursor-pointer" @click="router.push(`/user/${video.author.id}`)">
              <div class="text-sm font-medium text-ink truncate hover:text-primary transition">{{ video.author.username }}</div>
              <div class="text-xs text-ink-muted">{{ video.author.fans_count }} 粉丝</div>
            </div>
            <!-- 关注按钮 -->
            <button
              class="flex items-center gap-1 px-4 h-8 rounded text-sm font-medium transition shrink-0"
              :class="
                isFollowed
                  ? 'bg-surface-subtle text-ink-secondary hover:bg-surface-muted'
                  : 'bg-primary text-white hover:bg-primary-dark'
              "
              @click="toggleFollow"
            >
              <Bell :size="14" />
              {{ isFollowed ? '已关注' : '关注' }}
            </button>
          </div>

          <!-- 分隔线 -->
          <div class="my-3 border-t border-surface-subtle" />

          <!-- 互动工具栏（B站风格：圆角按钮） -->
          <div class="flex items-center gap-2 flex-wrap">
            <button
              class="flex items-center gap-1.5 px-4 h-8 rounded-full text-sm font-medium transition"
              :class="
                isLiked
                  ? 'bg-primary/10 text-primary'
                  : 'bg-surface-subtle text-ink-secondary hover:bg-surface-muted'
              "
              @click="toggleLike"
            >
              <Heart :size="18" :fill="isLiked ? 'currentColor' : 'none'" />
              <span>{{ video.likes_count }}</span>
            </button>
            <button
              class="flex items-center gap-1.5 px-4 h-8 rounded-full text-sm font-medium bg-surface-subtle text-ink-secondary hover:bg-surface-muted transition"
              @click="showCoinModal = true"
            >
              <Coins :size="18" />
              <span>{{ video.coin_count }}</span>
            </button>
            <button
              class="flex items-center gap-1.5 px-4 h-8 rounded-full text-sm font-medium transition"
              :class="
                isFavorited
                  ? 'bg-secondary/10 text-secondary'
                  : 'bg-surface-subtle text-ink-secondary hover:bg-surface-muted'
              "
              @click="toggleFavorite"
            >
              <Star :size="18" :fill="isFavorited ? 'currentColor' : 'none'" />
              <span>{{ video.fav_count }}</span>
            </button>
            <button
              class="flex items-center gap-1.5 px-4 h-8 rounded-full text-sm font-medium bg-surface-subtle text-ink-secondary hover:bg-surface-muted transition"
            >
              <Share2 :size="18" />
              <span>分享</span>
            </button>
          </div>
        </div>

        <!-- 视频简介 -->
        <div v-if="video.description" class="mt-3 p-4 bg-white rounded-card">
          <h3 class="text-sm font-medium text-ink mb-2">视频简介</h3>
          <p class="text-sm text-ink-secondary whitespace-pre-wrap break-words leading-relaxed">
            {{ video.description }}
          </p>
        </div>

        <!-- 评论区 -->
        <div class="mt-4 p-4 bg-white rounded-card">
          <h3 class="text-base font-medium text-ink mb-4 flex items-center gap-2">
            <span>评论</span>
            <span class="text-sm text-ink-muted">{{ video.comment_count }}</span>
          </h3>
          <CommentTree
            v-if="!video.comments_closed"
            :video-id="videoId"
            comment-type="video"
          />
          <div v-else class="py-8 text-center text-sm text-ink-muted">评论已关闭</div>
        </div>
      </div>

      <!-- 右侧推荐 -->
      <div class="w-[320px] shrink-0 space-y-3 hidden lg:block">
        <h3 class="text-sm font-medium text-ink flex items-center gap-2">
          <span class="w-1 h-4 bg-secondary rounded-full" />
          推荐视频
        </h3>
        <VideoCard v-for="rec in recommendList" :key="rec.id" :video="rec" />
        <EmptyState v-if="!recommendList.length" text="暂无推荐" />
      </div>
    </div>

    <EmptyState v-else text="视频不存在或已下架" />

    <!-- 投币弹窗 -->
    <div
      v-if="showCoinModal"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      @click.self="showCoinModal = false"
    >
      <div class="bg-white rounded-xl p-6 w-80 animate-fade-in">
        <div class="flex items-center justify-between mb-4">
          <h3 class="text-base font-medium text-ink">投币</h3>
          <button class="text-ink-muted hover:text-ink" @click="showCoinModal = false">
            <X :size="18" />
          </button>
        </div>
        <div class="space-y-2">
          <button
            class="w-full flex items-center justify-between px-4 py-3 rounded-lg border-2 transition"
            :class="
              coinAmount === 1
                ? 'border-primary text-primary bg-primary/5'
                : 'border-surface-muted text-ink-secondary hover:border-primary/50'
            "
            @click="coinAmount = 1"
          >
            <span class="text-sm">投 1 枚硬币</span>
            <Coins :size="20" />
          </button>
          <button
            class="w-full flex items-center justify-between px-4 py-3 rounded-lg border-2 transition"
            :class="
              coinAmount === 2
                ? 'border-primary text-primary bg-primary/5'
                : 'border-surface-muted text-ink-secondary hover:border-primary/50'
            "
            @click="coinAmount = 2"
          >
            <span class="text-sm">投 2 枚硬币</span>
            <div class="flex gap-1">
              <Coins :size="20" />
              <Coins :size="20" />
            </div>
          </button>
        </div>
        <button
          class="mt-4 w-full h-10 rounded-full bg-primary text-white text-sm font-medium hover:bg-primary-dark transition"
          @click="doCoin"
        >
          确认投币
        </button>
      </div>
    </div>
  </div>
</template>
