<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getHomeVideoList } from '@/api/video'
import type { HomeVideoInfo } from '@/types'
import VideoCard from '@/components/common/VideoCard.vue'
import VideoCarousel from '@/components/common/VideoCarousel.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { RefreshCw as Refresh } from 'lucide-vue-next'

const route = useRoute()
const router = useRouter()

// 当前选中的分区（空字符串=推荐，即用户进入首页默认看到的推荐内容）
const activeZone = computed(() => (route.query.zone as string) || '')

// 当前分区标题（默认展示“推荐”内容）
const currentZoneLabel = computed(() => activeZone.value || '推荐')

// 视频列表（游标分页）
const videos = ref<HomeVideoInfo[]>([])
const cursor = ref('')
const hasMore = ref(false)
const loading = ref(false)
const refreshing = ref(false)

// 热门轮播：当前热度最高的 5 个视频（全局，与分区无关；不足 5 个则有几个算几个）
const carouselVideos = ref<HomeVideoInfo[]>([])

async function loadCarousel() {
  try {
    const res = await getHomeVideoList('', 5, '')
    carouselVideos.value = res.list || []
  } catch {
    carouselVideos.value = []
  }
}

async function loadFirst() {
  cursor.value = ''
  videos.value = []
  loading.value = true
  try {
    const res = await getHomeVideoList('', 30, activeZone.value)
    videos.value = res.list || []
    cursor.value = res.next_cursor
    hasMore.value = res.has_more
  } catch {
    videos.value = []
    hasMore.value = false
  } finally {
    loading.value = false
  }
}

async function loadMore() {
  if (loading.value || !hasMore.value) return
  loading.value = true
  try {
    const res = await getHomeVideoList(cursor.value, 30, activeZone.value)
    videos.value.push(...(res.list || []))
    cursor.value = res.next_cursor
    hasMore.value = res.has_more
  } catch {
    hasMore.value = false
  } finally {
    loading.value = false
  }
}

async function refresh() {
  refreshing.value = true
  await loadFirst()
  refreshing.value = false
}

// 无限滚动
function handleScroll() {
  const scrollTop = window.pageYOffset || document.documentElement.scrollTop
  const scrollHeight = document.documentElement.scrollHeight
  const clientHeight = document.documentElement.clientHeight
  if (scrollHeight - scrollTop - clientHeight < 300) {
    loadMore()
  }
}

onMounted(() => {
  loadFirst()
  loadCarousel()
  window.addEventListener('scroll', handleScroll, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', handleScroll)
})

// 分区切换时重新加载
watch(activeZone, () => {
  loadFirst()
})

function formatCount(n: number): string {
  if (n >= 10000) return (n / 10000).toFixed(1) + '万'
  return String(n)
}
</script>

<template>
  <div class="max-w-[1280px] mx-auto px-4 py-4 min-[1440px]:max-w-none">
    <!-- 分区标题栏 -->
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-lg font-medium text-ink flex items-center gap-2">
        <span class="w-1 h-5 bg-primary rounded-full" />
        {{ currentZoneLabel }}
        <span class="text-xs text-ink-muted font-normal ml-1">{{ videos.length }} 个视频</span>
      </h2>
      <button
        class="flex items-center gap-1 text-xs text-ink-muted hover:text-primary transition"
        :disabled="refreshing"
        @click="refresh"
      >
        <Refresh :size="14" :class="{ 'animate-spin': refreshing }" />
        刷新
      </button>
    </div>

    <!-- 骨架屏 -->
    <div v-if="loading && !videos.length" class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
      <div v-for="i in 15" :key="i" class="bg-white rounded-card overflow-hidden">
        <div class="aspect-video bg-surface-muted animate-pulse" />
        <div class="p-2 space-y-2">
          <div class="h-4 bg-surface-muted rounded animate-pulse" />
          <div class="h-3 bg-surface-muted rounded animate-pulse w-2/3" />
        </div>
      </div>
    </div>

    <!-- 视频网格：轮播图占左上 2 列 × 2 行，其余为视频卡片 -->
    <!-- auto-rows-fr 让每一行等高，row-span-2 才能精确占据 2 行高度 -->
    <div v-else-if="videos.length" class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 auto-rows-fr gap-5">
      <VideoCarousel
        v-if="carouselVideos.length"
        :videos="carouselVideos"
        class="col-span-2 row-span-2 self-stretch h-full"
      />
      <VideoCard v-for="video in videos" :key="video.id" :video="video" />
    </div>

    <EmptyState v-else :text="`暂无${currentZoneLabel}视频`" />

    <!-- 加载更多 -->
    <div v-if="loading && videos.length" class="py-6 text-center text-sm text-ink-muted">
      加载中...
    </div>
    <div v-else-if="!hasMore && videos.length" class="py-6 text-center text-xs text-ink-muted">
      没有更多了
    </div>
  </div>
</template>
