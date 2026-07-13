<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { X, Trash2, History } from 'lucide-vue-next'
import { searchVideo } from '@/api/search'
import {
  getSearchHistory,
  deleteSearchHistory,
  clearSearchHistory,
  saveSearchHistory,
} from '@/api/history'
import type { HomeVideoInfo, SearchHistoryItem } from '@/types'
import VideoCard from '@/components/common/VideoCard.vue'
import EmptyState from '@/components/common/EmptyState.vue'

const route = useRoute()
const router = useRouter()

const keyword = ref('')
const videos = ref<HomeVideoInfo[]>([])
const cursor = ref('')
const hasMore = ref(true)
const loading = ref(false)
const searched = ref(false)

const history = ref<SearchHistoryItem[]>([])

async function loadHistory() {
  try {
    history.value = await getSearchHistory()
  } catch {
    // ignore
  }
}

async function doSearch(reset = false) {
  if (!keyword.value || loading.value) return
  if (!reset && !hasMore.value) return
  loading.value = true
  try {
    const res = await searchVideo(keyword.value, reset ? '' : cursor.value, 20)
    const newList = res.list || []
    if (reset) videos.value = newList
    else videos.value.push(...newList)
    cursor.value = res.next_cursor
    hasMore.value = res.has_more
    searched.value = true
  } finally {
    loading.value = false
  }
}

async function triggerSearch(reset: boolean) {
  if (!keyword.value) return
  if (reset) {
    saveSearchHistory(keyword.value).then(loadHistory).catch(() => {})
  }
  await doSearch(reset)
}

function handleScroll() {
  if (window.innerHeight + window.scrollY >= document.body.offsetHeight - 200) {
    doSearch()
  }
}

function searchHistoryKeyword(kw: string) {
  router.push({ path: '/search', query: { keyword: kw } })
}

async function removeHistory(keyword: string) {
  try {
    await deleteSearchHistory(keyword)
    history.value = history.value.filter((h) => h.keyword !== keyword)
  } catch {
    // ignore
  }
}

async function removeAllHistory() {
  try {
    await clearSearchHistory()
    history.value = []
  } catch {
    // ignore
  }
}

watch(
  () => route.query.keyword,
  (kw) => {
    keyword.value = (kw as string) || ''
    videos.value = []
    cursor.value = ''
    hasMore.value = true
    searched.value = false
    if (keyword.value) triggerSearch(true)
  },
  { immediate: true }
)

onMounted(() => {
  loadHistory()
  window.addEventListener('scroll', handleScroll)
})

onUnmounted(() => {
  window.removeEventListener('scroll', handleScroll)
})
</script>

<template>
  <div class="max-w-[1280px] mx-auto min-[1440px]:max-w-none px-4 py-4">
    <!-- 搜索历史 -->
    <div v-if="history.length" class="mb-4 bg-white rounded-card shadow-card p-4">
      <div class="flex items-center justify-between mb-2">
        <div class="flex items-center gap-1.5 text-sm font-medium text-ink">
          <History :size="16" class="text-ink-secondary" />
          搜索历史
        </div>
        <button
          class="flex items-center gap-1 text-xs text-ink-muted hover:text-primary transition"
          @click="removeAllHistory"
        >
          <Trash2 :size="14" />
          清空
        </button>
      </div>
      <div class="flex flex-wrap gap-2">
        <div
          v-for="item in history"
          :key="item.keyword"
          class="flex items-center gap-0.5 pl-2.5 pr-1 py-1 bg-surface-subtle rounded-full text-xs text-ink-secondary hover:bg-primary/10 hover:text-primary transition"
        >
          <button class="hover:underline" @click="searchHistoryKeyword(item.keyword)">
            {{ item.keyword }}
          </button>
          <button
            class="p-0.5 rounded-full hover:bg-primary/20"
            title="删除"
            @click="removeHistory(item.keyword)"
          >
            <X :size="12" />
          </button>
        </div>
      </div>
    </div>

    <!-- 搜索结果标题 -->
    <div v-if="keyword" class="mb-3 text-sm text-ink-secondary">
      搜索 "<span class="text-ink font-medium">{{ keyword }}</span>" 的结果
    </div>

    <!-- 视频网格 -->
    <div
      v-if="videos.length"
      class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4"
    >
      <VideoCard v-for="video in videos" :key="video.id" :video="video" />
    </div>

    <!-- 骨架屏 -->
    <div
      v-if="loading && !videos.length"
      class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4"
    >
      <div v-for="i in 10" :key="i" class="bg-white rounded-card overflow-hidden">
        <div class="aspect-video bg-surface-muted animate-pulse" />
        <div class="p-2 space-y-2">
          <div class="h-4 bg-surface-muted rounded animate-pulse" />
          <div class="h-3 bg-surface-muted rounded animate-pulse w-2/3" />
        </div>
      </div>
    </div>

    <!-- 加载更多 -->
    <div v-if="loading && videos.length" class="py-6 text-center text-sm text-ink-muted">
      加载中...
    </div>
    <div v-if="!loading && !hasMore && videos.length" class="py-6 text-center text-sm text-ink-muted">
      没有更多了
    </div>

    <!-- 空状态 -->
    <EmptyState
      v-if="!loading && searched && !videos.length"
      :text="`没有找到与 &quot;${keyword}&quot; 相关的视频`"
    />
    <EmptyState v-if="!keyword && !history.length" text="输入关键词开始搜索吧" />
  </div>
</template>
