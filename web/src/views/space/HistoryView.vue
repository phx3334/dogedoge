<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Trash2, Search } from 'lucide-vue-next'
import {
  getVideoHistory,
  deleteVideoHistory,
  clearVideoHistory,
  getArticleHistory,
  deleteArticleHistory,
  getSearchHistory,
  deleteSearchHistory,
  clearSearchHistory,
} from '@/api/history'
import type { VideoHistoryItem, ArticleHistoryItem, SearchHistoryItem, PaginatedResp } from '@/types'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'

const router = useRouter()
const tab = ref<'video' | 'article' | 'search'>('video')
const pageSize = 10

// 视频历史
const videoList = ref<VideoHistoryItem[]>([])
const videoPage = ref(1)
const videoTotal = ref(0)
const loadingVideo = ref(false)

// 文章历史
const articleList = ref<ArticleHistoryItem[]>([])
const articlePage = ref(1)
const articleTotal = ref(0)
const loadingArticle = ref(false)

// 搜索历史
const searchList = ref<SearchHistoryItem[]>([])
const loadingSearch = ref(false)

function fmtDate(d: string) {
  try {
    return new Date(d).toLocaleDateString()
  } catch {
    return d
  }
}

function fmtDuration(s: number) {
  const min = Math.floor(s / 60)
  const sec = Math.floor(s % 60)
  return `${min}:${String(sec).padStart(2, '0')}`
}

function progress(item: VideoHistoryItem) {
  // 修复 NaN：后端返回字段为 progress_sec 和 duration_sec（或 duration）
  const watched = item.progress_sec || 0
  const dur = item.duration || item.duration_sec || 0
  if (!dur || dur <= 0) return 0
  return Math.min(100, Math.round((watched / dur) * 100))
}

async function loadVideo() {
  loadingVideo.value = true
  try {
    const res: PaginatedResp<VideoHistoryItem> = await getVideoHistory(videoPage.value, pageSize)
    videoList.value = res.list
    videoTotal.value = res.total
  } finally {
    loadingVideo.value = false
  }
}

async function loadArticle() {
  loadingArticle.value = true
  try {
    const res: PaginatedResp<ArticleHistoryItem> = await getArticleHistory(articlePage.value, pageSize)
    articleList.value = res.list || []
    articleTotal.value = res.total || 0
  } finally {
    loadingArticle.value = false
  }
}

async function loadSearch() {
  loadingSearch.value = true
  try {
    searchList.value = await getSearchHistory()
  } finally {
    loadingSearch.value = false
  }
}

async function ensureLoaded() {
  if (tab.value === 'video' && !videoList.value.length && !loadingVideo.value) await loadVideo()
  else if (tab.value === 'article' && !articleList.value.length && !loadingArticle.value) await loadArticle()
  else if (tab.value === 'search' && !searchList.value.length && !loadingSearch.value) await loadSearch()
}

async function removeVideo(item: VideoHistoryItem, e: Event) {
  e.stopPropagation()
  await deleteVideoHistory(item.id)
  if (videoList.value.length === 1 && videoPage.value > 1) videoPage.value--
  await loadVideo()
}

async function removeAllVideo() {
  if (!confirm('确认清空全部观看历史？')) return
  await clearVideoHistory()
  videoPage.value = 1
  await loadVideo()
}

function goVideo(item: VideoHistoryItem) {
  // 传递观看进度，视频详情页读取后自动定位到上次观看位置
  const t = item.progress_sec || 0
  router.push({ path: `/video/${item.video_id}`, query: t > 0 ? { t: String(t) } : {} })
}

async function onVideoPage(p: number) {
  videoPage.value = p
  await loadVideo()
  window.scrollTo({ top: 0 })
}

async function removeArticle(item: ArticleHistoryItem, e: Event) {
  e.stopPropagation()
  await deleteArticleHistory(item.id)
  if (articleList.value.length === 1 && articlePage.value > 1) articlePage.value--
  await loadArticle()
}

async function onArticlePage(p: number) {
  articlePage.value = p
  await loadArticle()
  window.scrollTo({ top: 0 })
}

function goArticle(item: ArticleHistoryItem) {
  router.push(`/article/${item.article_id}`)
}

async function removeSearch(item: SearchHistoryItem, e: Event) {
  e.stopPropagation()
  await deleteSearchHistory(item.id)
  await loadSearch()
}

async function removeAllSearch() {
  if (!confirm('确认清空全部搜索历史？')) return
  await clearSearchHistory()
  await loadSearch()
}

function goSearch(keyword: string) {
  router.push({ path: '/search', query: { keyword } })
}

watch(tab, () => ensureLoaded())

onMounted(() => {
  loadVideo()
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-bold text-ink">历史记录</h2>
      <button
        v-if="tab === 'video' && videoList.length"
        class="flex items-center gap-1 px-3 h-8 rounded text-sm text-ink-secondary hover:text-primary hover:bg-surface-subtle transition"
        @click="removeAllVideo"
      >
        <Trash2 :size="14" /> 清空全部
      </button>
      <button
        v-else-if="tab === 'search' && searchList.length"
        class="flex items-center gap-1 px-3 h-8 rounded text-sm text-ink-secondary hover:text-primary hover:bg-surface-subtle transition"
        @click="removeAllSearch"
      >
        <Trash2 :size="14" /> 清空全部
      </button>
    </div>

    <!-- Tab 切换 -->
    <div class="flex items-center gap-1 bg-white rounded-card shadow-card p-1 w-fit">
      <button
        v-for="t in [{k:'video',l:'视频'},{k:'article',l:'文章'},{k:'search',l:'搜索'}]"
        :key="t.k"
        class="px-4 h-8 rounded text-sm transition"
        :class="tab === t.k ? 'bg-primary text-white' : 'text-ink-secondary hover:bg-surface-subtle'"
        @click="tab = t.k as 'video' | 'article' | 'search'"
      >
        {{ t.l }}
      </button>
    </div>

    <div class="bg-white rounded-card shadow-card p-4">
      <!-- 视频历史 -->
      <template v-if="tab === 'video'">
        <div v-if="loadingVideo" class="py-12 text-center text-sm text-ink-muted">加载中...</div>
        <template v-else-if="videoList.length">
          <div class="space-y-3">
            <div
              v-for="item in videoList"
              :key="item.id"
              class="flex gap-3 p-2 rounded hover:bg-surface-subtle cursor-pointer transition group"
              @click="goVideo(item)"
            >
              <div class="relative w-40 aspect-video bg-surface-muted rounded overflow-hidden shrink-0">
                <img :src="item.cover_url" :alt="item.title" loading="lazy" class="w-full h-full object-cover" />
                <div class="absolute bottom-1 right-1 px-1.5 py-0.5 bg-black/70 text-white text-xs rounded">
                  {{ fmtDuration(item.duration) }}
                </div>
              </div>
              <div class="flex-1 min-w-0 flex flex-col py-1">
                <h3 class="text-sm text-ink line-clamp-2">{{ item.title }}</h3>
                <p class="text-xs text-ink-muted mt-1">UP主：{{ item.up_name }}</p>
                <p class="text-xs text-ink-muted">观看时间：{{ fmtDate(item.viewed_at) }}</p>
                <!-- 进度条 -->
                <div class="mt-auto pt-2">
                  <div class="h-1 bg-surface-muted rounded-full overflow-hidden">
                    <div class="h-full bg-primary" :style="{ width: progress(item) + '%' }"></div>
                  </div>
                  <p class="text-xs text-ink-muted mt-1">已观看 {{ progress(item) }}%</p>
                </div>
              </div>
              <button
                class="self-start p-1.5 text-ink-muted hover:text-primary opacity-0 group-hover:opacity-100 transition"
                title="删除"
                @click="removeVideo(item, $event)"
              >
                <Trash2 :size="16" />
              </button>
            </div>
          </div>
          <Pagination :current="videoPage" :total="videoTotal" :page-size="pageSize" @change="onVideoPage" />
        </template>
        <EmptyState v-else text="暂无观看历史" />
      </template>

      <!-- 文章历史 -->
      <template v-else-if="tab === 'article'">
        <div v-if="loadingArticle" class="py-12 text-center text-sm text-ink-muted">加载中...</div>
        <template v-else-if="articleList.length">
          <div class="space-y-3">
            <div
              v-for="item in articleList"
              :key="item.id"
              class="flex gap-3 p-2 rounded hover:bg-surface-subtle cursor-pointer transition group"
              @click="goArticle(item)"
            >
              <div class="w-24 h-16 bg-surface-muted rounded overflow-hidden shrink-0">
                <img :src="item.cover_url" :alt="item.title" loading="lazy" class="w-full h-full object-cover" />
              </div>
              <div class="flex-1 min-w-0 py-1">
                <h3 class="text-sm text-ink line-clamp-2">{{ item.title }}</h3>
                <p class="text-xs text-ink-muted mt-1">阅读时间：{{ fmtDate(item.viewed_at) }}</p>
              </div>
              <button
                class="self-start p-1.5 text-ink-muted hover:text-primary opacity-0 group-hover:opacity-100 transition"
                title="删除"
                @click="removeArticle(item, $event)"
              >
                <Trash2 :size="16" />
              </button>
            </div>
          </div>
          <Pagination :current="articlePage" :total="articleTotal" :page-size="pageSize" @change="onArticlePage" />
        </template>
        <EmptyState v-else text="暂无文章阅读历史" />
      </template>

      <!-- 搜索历史 -->
      <template v-else>
        <div v-if="loadingSearch" class="py-12 text-center text-sm text-ink-muted">加载中...</div>
        <template v-else-if="searchList.length">
          <div class="flex flex-wrap gap-2">
            <div
              v-for="item in searchList"
              :key="item.id"
              class="group flex items-center gap-1 pl-3 pr-1 h-8 bg-surface-subtle hover:bg-surface-muted rounded-full cursor-pointer transition"
              @click="goSearch(item.keyword)"
            >
              <Search :size="13" class="text-ink-muted" />
              <span class="text-sm text-ink">{{ item.keyword }}</span>
              <span class="text-xs text-ink-muted">{{ fmtDate(item.searched_at) }}</span>
              <button
                class="w-6 h-6 flex items-center justify-center rounded-full text-ink-muted hover:text-primary hover:bg-white transition"
                title="删除"
                @click="removeSearch(item, $event)"
              >
                <Trash2 :size="12" />
              </button>
            </div>
          </div>
        </template>
        <EmptyState v-else text="暂无搜索历史" />
      </template>
    </div>
  </div>
</template>
