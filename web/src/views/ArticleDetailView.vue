<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useRoute } from 'vue-router'
import { Heart, Bookmark, Eye, Send, ThumbsUp, X } from 'lucide-vue-next'
import { getArticleDetail } from '@/api/article'
import { getCommentList, createComment, likeComment } from '@/api/comment'
import { useUserStore } from '@/stores/user'
import { useCommentScroll } from '@/composables/useCommentScroll'
import type { ArticleDetailResp, CommentItem } from '@/types'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'

const route = useRoute()
const userStore = useUserStore()

// 通知点击跳转后滚动并高亮对应评论
useCommentScroll()

const article = ref<ArticleDetailResp | null>(null)
const loading = ref(false)
const liked = ref(false)
const favorited = ref(false)

const comments = ref<CommentItem[]>([])
const commentPage = ref(1)
const commentTotal = ref(0)
const commentLoading = ref(false)
const commentText = ref('')
const submitting = ref(false)

const articleId = computed(() => Number(route.params.id))

// 解析文章图片列表（images_json 为 JSON 字符串数组）
const images = computed<string[]>(() => {
  if (!article.value?.images_json) return []
  try {
    const parsed = JSON.parse(article.value.images_json)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
})

// 图片预览
const previewSrc = ref<string>('')
const showPreview = ref(false)
function openPreview(src: string) {
  previewSrc.value = src
  showPreview.value = true
}
function closePreview() {
  showPreview.value = false
}

async function loadArticle() {
  loading.value = true
  try {
    const res = await getArticleDetail(articleId.value)
    article.value = res
    liked.value = res.is_liked
    favorited.value = res.is_favorited
  } finally {
    loading.value = false
  }
}

async function loadComments(page: number = 1) {
  commentLoading.value = true
  try {
    const res = await getCommentList('article', articleId.value, page, 20)
    comments.value = res.list || []
    commentTotal.value = res.total || 0
    commentPage.value = page
  } finally {
    commentLoading.value = false
  }
}

async function submitComment() {
  if (!userStore.isLogin) {
    alert('请先登录')
    return
  }
  if (!commentText.value.trim() || submitting.value) return
  submitting.value = true
  try {
    await createComment('article', articleId.value, commentText.value.trim(), 0)
    commentText.value = ''
    await loadComments(1)
    if (article.value) article.value.comment_count++
  } finally {
    submitting.value = false
  }
}

async function toggleLikeComment(c: CommentItem) {
  if (!userStore.isLogin) {
    alert('请先登录')
    return
  }
  try {
    await likeComment('article', c.id)
    c.like_count++
  } catch {
    // ignore
  }
}

function toggleLike() {
  if (!userStore.isLogin) {
    alert('请先登录')
    return
  }
  liked.value = !liked.value
  if (article.value) article.value.like_count += liked.value ? 1 : -1
}

function toggleFavorite() {
  if (!userStore.isLogin) {
    alert('请先登录')
    return
  }
  favorited.value = !favorited.value
}

function formatDate(s: string) {
  return new Date(s).toLocaleString('zh-CN')
}

watch(
  () => route.params.id,
  () => {
    if (articleId.value) {
      loadArticle()
      loadComments(1)
    }
  },
  { immediate: true }
)
</script>

<template>
  <div class="max-w-3xl mx-auto px-4 py-6">
    <!-- 加载中 -->
    <div v-if="loading" class="text-center py-20 text-sm text-ink-muted">加载中...</div>

    <article v-else-if="article" class="bg-white rounded-card shadow-card p-6 md:p-8">
      <!-- 标题 -->
      <h1 class="text-2xl font-bold text-ink leading-tight">{{ article.title }}</h1>

      <!-- 作者信息 -->
      <div class="flex items-center gap-3 mt-4 pb-4 border-b border-surface-muted">
        <img
          :src="article.author?.avatar_url || '/uploads/avatar/default.jpg'"
          :alt="article.author?.username"
          class="w-10 h-10 rounded-full object-cover"
          @error="($event.target as HTMLImageElement).src = '/uploads/avatar/default.jpg'"
        />
        <div class="flex-1">
          <div class="text-sm font-medium text-ink">{{ article.author?.username }}</div>
          <div class="text-xs text-ink-muted">{{ formatDate(article.created_at) }}</div>
        </div>
      </div>

      <!-- 标签 -->
      <div v-if="article.tags.length" class="flex flex-wrap gap-2 mt-4">
        <span
          v-for="tag in article.tags"
          :key="tag"
          class="px-2 py-0.5 text-xs bg-secondary/10 text-secondary rounded"
        >
          #{{ tag }}
        </span>
      </div>

      <!-- 正文 -->
      <div
        class="mt-4 text-ink leading-7 [&_p]:my-3 [&_img]:max-w-full [&_img]:rounded-lg [&_a]:text-secondary [&_a]:underline [&_ul]:list-disc [&_ul]:pl-5 [&_ol]:list-decimal [&_ol]:pl-5 [&_blockquote]:border-l-4 [&_blockquote]:border-surface-muted [&_blockquote]:pl-3 [&_blockquote]:text-ink-secondary"
        v-html="article.body_md"
      />

      <!-- 文章配图 -->
      <div v-if="images.length" class="mt-4 grid grid-cols-3 gap-2">
        <div
          v-for="(img, idx) in images"
          :key="idx"
          class="relative aspect-square rounded-lg overflow-hidden cursor-pointer group"
          @click="openPreview(img)"
        >
          <img
            :src="img"
            :alt="`配图${idx + 1}`"
            class="w-full h-full object-cover transition group-hover:scale-105"
            @error="($event.target as HTMLImageElement).style.display = 'none'"
          />
        </div>
      </div>

      <!-- 图片预览大图 -->
      <div
        v-if="showPreview"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/80"
        @click="closePreview"
      >
        <button class="absolute top-4 right-4 text-white/80 hover:text-white" @click="closePreview">
          <X :size="28" />
        </button>
        <img
          :src="previewSrc"
          class="max-w-[90vw] max-h-[90vh] object-contain rounded-lg"
          @click.stop
        />
      </div>

      <!-- 互动栏 -->
      <div class="flex items-center gap-6 mt-6 pt-4 border-t border-surface-muted">
        <button
          class="flex items-center gap-1.5 text-sm transition"
          :class="liked ? 'text-primary' : 'text-ink-secondary hover:text-primary'"
          @click="toggleLike"
        >
          <Heart :size="18" :fill="liked ? 'currentColor' : 'none'" />
          {{ article.like_count }}
        </button>
        <button
          class="flex items-center gap-1.5 text-sm transition"
          :class="favorited ? 'text-primary' : 'text-ink-secondary hover:text-primary'"
          @click="toggleFavorite"
        >
          <Bookmark :size="18" :fill="favorited ? 'currentColor' : 'none'" />
          收藏
        </button>
        <div class="flex items-center gap-1.5 text-sm text-ink-muted ml-auto">
          <Eye :size="18" />
          {{ article.view_count }}
        </div>
      </div>
    </article>

    <!-- 评论区 -->
    <section v-if="article" class="bg-white rounded-card shadow-card p-6 md:p-8 mt-4">
      <h2 class="text-lg font-bold text-ink mb-4">
        评论 <span class="text-sm text-ink-muted font-normal">{{ article.comment_count }}</span>
      </h2>

      <!-- 评论已关闭 -->
      <div v-if="article.comments_closed" class="py-8 text-center text-sm text-ink-muted">
        评论已关闭
      </div>

      <template v-else>
        <!-- 评论输入框 -->
        <div class="mb-6">
          <textarea
            v-model="commentText"
            rows="3"
            placeholder="写下你的评论..."
            class="w-full p-3 bg-surface-subtle rounded-lg text-sm resize-none focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
          />
          <div class="flex justify-end mt-2">
            <button
              class="flex items-center gap-1 px-4 h-8 bg-primary text-white rounded-lg text-sm hover:bg-primary-dark transition disabled:opacity-60"
              :disabled="!commentText.trim() || submitting"
              @click="submitComment"
            >
              <Send :size="14" />
              发表
            </button>
          </div>
        </div>

        <!-- 评论列表 -->
        <div v-if="commentLoading && !comments.length" class="text-center py-6 text-sm text-ink-muted">
          加载中...
        </div>
        <div v-else-if="comments.length" class="space-y-4">
          <div v-for="c in comments" :key="c.id" :data-comment-id="c.id" class="flex gap-3">
            <img
              :src="c.user?.avatar_url || '/uploads/avatar/default.jpg'"
              :alt="c.user?.username"
              class="w-8 h-8 rounded-full object-cover shrink-0"
              @error="($event.target as HTMLImageElement).src = '/uploads/avatar/default.jpg'"
            />
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium text-ink">{{ c.user?.username }}</div>
              <div class="text-sm text-ink-secondary mt-1 whitespace-pre-wrap break-words">
                {{ c.content }}
              </div>
              <div class="flex items-center gap-4 mt-2 text-xs text-ink-muted">
                <span>{{ formatDate(c.created_at) }}</span>
                <button
                  class="flex items-center gap-1 transition hover:text-primary"
                  @click="toggleLikeComment(c)"
                >
                  <ThumbsUp :size="14" />
                  {{ c.like_count }}
                </button>
              </div>
            </div>
          </div>
        </div>
        <EmptyState v-else text="还没有评论，快来抢沙发" />

        <Pagination
          v-if="comments.length"
          :current="commentPage"
          :total="commentTotal"
          :page-size="20"
          @change="(p) => loadComments(p)"
        />
      </template>
    </section>
  </div>
</template>
