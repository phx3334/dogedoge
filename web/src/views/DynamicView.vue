<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { getDynamicFeed, createDynamic, likeDynamic, unlikeDynamic } from '@/api/dynamic'
import { getUserHome } from '@/api/user'
import type { DynamicItem, UserHomeResp } from '@/types'
import { useUserStore } from '@/stores/user'
import EmptyState from '@/components/common/EmptyState.vue'
import Pagination from '@/components/common/Pagination.vue'
import { Heart, MessageCircle, ImagePlus, Send, Play, FileText, Smile, Users, Rss } from 'lucide-vue-next'

const router = useRouter()
const userStore = useUserStore()
const myAvatar = () => userStore.userInfo?.avatar_url || '/uploads/avatar/default.jpg'

// ===== 左侧个人卡片 =====
const homeData = ref<UserHomeResp | null>(null)
const myUserId = computed(() => userStore.userInfo?.id || '')

async function loadMyHome() {
  if (!userStore.isLogin || !myUserId.value) return
  try {
    homeData.value = await getUserHome(myUserId.value, 1, 1)
  } catch {
    homeData.value = null
  }
}

import { calcLevel } from '@/utils/level'

const myLevel = computed(() => {
  const exp = homeData.value?.experience ?? userStore.userInfo?.experience ?? 0
  return calcLevel(exp)
})

// ===== 发布动态 =====
const title = ref('')
const content = ref('')
const images = ref<string[]>([])
const publishing = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)

function handleFileSelect(e: Event) {
  const target = e.target as HTMLInputElement
  if (!target.files) return
  const remaining = 9 - images.value.length
  Array.from(target.files).slice(0, remaining).forEach((file) => {
    const reader = new FileReader()
    reader.onload = () => {
      if (typeof reader.result === 'string') images.value.push(reader.result)
    }
    reader.readAsDataURL(file)
  })
  target.value = ''
}

function removeImage(idx: number) {
  images.value.splice(idx, 1)
}

async function handlePublish() {
  if (!title.value.trim() || !content.value.trim() || publishing.value) return
  publishing.value = true
  try {
    await createDynamic(title.value.trim(), content.value.trim(), images.value)
    title.value = ''
    content.value = ''
    images.value = []
    await loadFeed(true)
  } catch (err: any) {
    alert(err?.message || '发布失败')
  } finally {
    publishing.value = false
  }
}

// ===== Feed 列表 =====
const list = ref<DynamicItem[]>([])
const page = ref(1)
const pageSize = 10
const total = ref(0)
const loading = ref(false)
const activeTab = ref<'all' | 'dynamic' | 'video' | 'article'>('all')

const filteredList = computed(() => {
  if (activeTab.value === 'all') return list.value.filter((item) => item.type !== 'article')
  return list.value.filter((item) => item.type === activeTab.value)
})

const tabs = [
  { key: 'all' as const, label: '全部' },
  { key: 'video' as const, label: '视频' },
  { key: 'article' as const, label: '文章' },
]

async function loadFeed(reset = false) {
  if (loading.value) return
  loading.value = true
  try {
    const p = reset ? 1 : page.value
    const res = await getDynamicFeed(p, pageSize)
    list.value = res.list || []
    total.value = res.total || 0
    page.value = res.page
  } catch (err: any) {
    alert(err?.message || '加载动态失败')
  } finally {
    loading.value = false
  }
}

function onPageChange(p: number) {
  page.value = p
  loadFeed()
}

async function toggleLike(item: DynamicItem) {
  if (item.type !== 'dynamic') return
  if (!userStore.isLogin) {
    router.push('/login')
    return
  }
  try {
    if (item.is_liked) {
      await unlikeDynamic(item.id)
      item.is_liked = false
      item.like_count--
    } else {
      await likeDynamic(item.id)
      item.is_liked = true
      item.like_count++
    }
  } catch (err: any) {
    alert(err?.message || '操作失败')
  }
}

function parseImages(json: string): string[] {
  if (!json) return []
  try {
    return JSON.parse(json)
  } catch {
    return []
  }
}

function formatTime(iso: string) {
  const d = new Date(iso)
  const diff = (Date.now() - d.getTime()) / 1000
  if (diff < 60) return '刚刚'
  if (diff < 3600) return Math.floor(diff / 60) + '分钟前'
  if (diff < 86400) return Math.floor(diff / 3600) + '小时前'
  if (diff < 2592000) return Math.floor(diff / 86400) + '天前'
  return `${d.getMonth() + 1}-${String(d.getDate()).padStart(2, '0')}`
}

function formatDuration(d: number) {
  const min = Math.floor(d / 60)
  const sec = Math.floor(d % 60)
  return `${min}:${String(sec).padStart(2, '0')}`
}

function formatCount(n: number) {
  if (n >= 10000) return (n / 10000).toFixed(1) + '万'
  return String(n)
}

function goVideo(videoId: number) {
  router.push(`/video/${videoId}`)
}

function goArticle(articleId: number) {
  router.push(`/article/${articleId}`)
}

function goUser(userId: string) {
  router.push(`/user/${userId}`)
}

onMounted(() => {
  loadMyHome()
  loadFeed(true)
})
</script>

<template>
  <div class="max-w-[1280px] mx-auto min-[1440px]:max-w-none px-4 py-4">
    <div class="dyn-grid">
      <!-- ============ 左侧个人卡片 ============ -->
      <aside class="dyn-left">
        <div class="bg-white rounded-card border border-surface-subtle p-4">
          <!-- 头像 + 用户名 + 等级 -->
          <div class="flex items-start gap-3 mb-3">
            <img
              :src="myAvatar()"
              alt="me"
              class="w-12 h-12 rounded-full object-cover border border-surface-subtle shrink-0 cursor-pointer"
              @error="($event.target as HTMLImageElement).src = '/uploads/avatar/default.jpg'"
              @click="myUserId && goUser(myUserId)"
            />
            <div class="flex-1 min-w-0 pt-1">
              <div
                class="text-[15px] font-semibold text-primary truncate cursor-pointer hover:text-primary-dark transition"
                @click="myUserId && goUser(myUserId)"
              >
                {{ userStore.userInfo?.username || '游客' }}
              </div>
              <span
                class="inline-block mt-1 px-2 py-0.5 rounded text-xs font-medium text-white bg-gradient-to-r from-secondary to-primary"
              >
                LV{{ myLevel }}
              </span>
            </div>
          </div>

          <!-- 数据统计 -->
          <div class="grid grid-cols-3 gap-1 pt-3 border-t border-surface-subtle">
            <div class="text-center cursor-pointer hover:text-secondary transition">
              <div class="text-base font-semibold text-ink tabular-nums">{{ formatCount(homeData?.following_count || 0) }}</div>
              <div class="text-xs text-ink-muted">关注</div>
            </div>
            <div class="text-center cursor-pointer hover:text-secondary transition">
              <div class="text-base font-semibold text-ink tabular-nums">{{ formatCount(homeData?.fans_count || 0) }}</div>
              <div class="text-xs text-ink-muted">粉丝</div>
            </div>
            <div class="text-center cursor-pointer hover:text-secondary transition">
              <div class="text-base font-semibold text-ink tabular-nums">{{ formatCount(homeData?.video_count || 0) }}</div>
              <div class="text-xs text-ink-muted">投稿</div>
            </div>
          </div>
        </div>

        <!-- 关注/粉丝快捷入口 -->
        <div class="bg-white rounded-card border border-surface-subtle mt-3 p-2">
          <router-link
            v-if="myUserId"
            :to="`/user/${myUserId}`"
            class="flex items-center gap-2 px-3 h-9 rounded text-sm text-ink-secondary hover:bg-surface-subtle transition"
          >
            <Rss :size="16" />
            我的主页
          </router-link>
          <router-link
            v-if="myUserId"
            :to="`/space/following?user_id=${myUserId}`"
            class="flex items-center gap-2 px-3 h-9 rounded text-sm text-ink-secondary hover:bg-surface-subtle transition"
          >
            <Users :size="16" />
            我的好友
          </router-link>
        </div>
      </aside>

      <!-- ============ 中间主区域 ============ -->
      <main class="dyn-center">
        <!-- 发布动态卡片（B站风格） -->
        <section v-if="userStore.isLogin" class="bg-white rounded-card border border-surface-subtle p-4">
          <div class="flex gap-3">
            <img
              :src="myAvatar()"
              alt="me"
              class="w-10 h-10 rounded-full object-cover shrink-0 cursor-pointer border border-surface-subtle"
              @error="($event.target as HTMLImageElement).src = '/uploads/avatar/default.jpg'"
              @click="myUserId && goUser(myUserId)"
            />
            <div class="flex-1 min-w-0 border border-surface-subtle rounded-lg p-3 bg-white">
            <input
              v-model="title"
              type="text"
              placeholder="好的标题更容易获得支持，选填20字"
              maxlength="20"
              class="block w-full px-0 border-none text-sm font-semibold text-ink bg-transparent outline-none placeholder:text-ink-muted mb-1.5"
            />
            <textarea
              v-model="content"
              placeholder="有什么想和大家分享的？"
              rows="3"
              maxlength="233"
              class="block w-full px-0 border-none text-sm text-ink bg-transparent outline-none placeholder:text-ink-muted resize-none min-h-[56px]"
            />

            <!-- 图片预览网格 -->
            <div v-if="images.length" class="flex flex-wrap gap-2 mt-2.5">
              <div
                v-for="(img, idx) in images"
                :key="idx"
                class="relative w-24 h-24 rounded-lg overflow-hidden bg-surface-muted"
              >
                <img :src="img" alt="preview" class="w-full h-full object-cover" />
                <button
                  class="absolute top-1 right-1 w-5 h-5 rounded-full bg-black/55 text-white text-sm leading-none flex items-center justify-center hover:bg-black/75"
                  @click="removeImage(idx)"
                >
                  ×
                </button>
              </div>
            </div>

            <!-- 工具栏 + 发布按钮 -->
            <div class="flex items-center justify-between mt-2.5">
              <div class="flex items-center gap-4">
                <button
                  type="button"
                  class="p-1 rounded text-ink-muted hover:text-secondary transition"
                  title="表情"
                >
                  <Smile :size="20" />
                </button>
                <button
                  type="button"
                  class="p-1 rounded text-ink-muted hover:text-secondary transition"
                  :class="{ 'text-secondary bg-secondary/10': images.length }"
                  title="图片"
                  @click="fileInput?.click()"
                >
                  <ImagePlus :size="20" />
                </button>
              </div>
              <div class="flex items-center gap-3">
                <span class="text-xs text-ink-muted tabular-nums">{{ content.length }}/233</span>
                <button
                  type="button"
                  :disabled="!title.trim() || !content.trim() || publishing"
                  class="min-w-[72px] h-8 px-4 rounded bg-secondary text-white text-sm font-medium hover:bg-secondary-dark transition disabled:opacity-45 disabled:cursor-not-allowed flex items-center justify-center gap-1"
                  @click="handlePublish"
                >
                  <Send :size="14" />
                  {{ publishing ? '发布中…' : '发布' }}
                </button>
              </div>
            </div>
            </div>
          </div>
          <input
            ref="fileInput"
            type="file"
            multiple
            accept="image/*"
            class="hidden"
            @change="handleFileSelect"
          />
        </section>

        <!-- 未登录提示 -->
        <section v-else class="bg-white rounded-card border border-surface-subtle p-7 text-center text-sm text-ink-secondary shadow-card">
          请先
          <router-link to="/login" class="text-secondary hover:underline">登录</router-link>
          后查看和发布动态。
        </section>

        <!-- Tab 筛选栏（B站风格） -->
        <nav class="dyn-tabs">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            type="button"
            class="relative px-0 py-2 text-[15px] bg-transparent border-none cursor-pointer leading-tight transition"
            :class="activeTab === tab.key ? 'text-secondary font-semibold' : 'text-ink-secondary hover:text-ink'"
            @click="activeTab = tab.key"
          >
            {{ tab.label }}
            <span
              v-if="activeTab === tab.key"
              class="absolute left-1/2 bottom-0.5 w-5 h-[3px] -ml-2.5 rounded bg-secondary"
            />
          </button>
        </nav>

        <!-- 骨架屏 -->
        <div v-if="loading && !list.length" class="space-y-3">
          <div v-for="i in 3" :key="i" class="bg-white rounded-card border border-surface-subtle p-4">
            <div class="flex items-start gap-3 mb-3">
              <div class="w-12 h-12 rounded-full bg-surface-muted animate-pulse shrink-0" />
              <div class="flex-1 space-y-1.5 pt-2">
                <div class="h-3.5 bg-surface-muted rounded w-24 animate-pulse" />
                <div class="h-2.5 bg-surface-muted rounded w-16 animate-pulse" />
              </div>
            </div>
            <div class="ml-[60px] space-y-2">
              <div class="h-4 bg-surface-muted rounded w-1/2 animate-pulse" />
              <div class="h-3 bg-surface-muted rounded w-full animate-pulse" />
              <div class="h-3 bg-surface-muted rounded w-4/5 animate-pulse" />
            </div>
          </div>
        </div>

        <!-- 动态列表 -->
        <div v-else-if="filteredList.length" class="space-y-3">
          <article
            v-for="item in filteredList"
            :key="`${item.type}-${item.id}`"
            class="bg-white rounded-card border border-surface-subtle p-4 animate-fade-in"
          >
            <!-- 卡片头部：头像 + 用户名 + 时间 + 类型标签 -->
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
                  <span>{{ formatTime(item.created_at) }}</span>
                  <span>·</span>
                  <span v-if="item.type === 'video'">投稿了视频</span>
                  <span v-else-if="item.type === 'article'">投稿了文章</span>
                  <span v-else>发布了动态</span>
                </div>
              </div>
              <!-- 类型徽章 -->
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

            <!-- 卡片正文（与头像右侧昵称/时间左缘对齐） -->
            <div class="ml-[60px] min-w-0">
              <!-- 视频类型 -->
              <template v-if="item.type === 'video'">
                <div
                  class="block cursor-pointer group"
                  @click="goVideo(item.video_id!)"
                >
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
                <div
                  class="block cursor-pointer group"
                  @click="goArticle(item.article_id!)"
                >
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

                <!-- 图片网格（B站风格） -->
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

              <!-- 底部操作栏（B站风格） -->
              <div class="flex items-center gap-5 mt-3 pt-3 border-t border-surface-subtle text-sm text-ink-secondary">
                <button
                  v-if="item.type === 'dynamic'"
                  class="flex items-center gap-1.5 transition hover:text-primary"
                  :class="{ 'text-primary': item.is_liked }"
                  @click="toggleLike(item)"
                >
                  <Heart
                    :size="18"
                    :fill="item.is_liked ? 'currentColor' : 'none'"
                    :class="{ 'animate-heart-pop': item.is_liked }"
                  />
                  <span>{{ item.like_count }}</span>
                </button>
                <div class="flex items-center gap-1.5">
                  <MessageCircle :size="18" />
                  <span>{{ item.comment_count }}</span>
                </div>
                <span v-if="item.type === 'video'" class="text-xs text-ink-muted ml-auto">
                  播放 {{ formatCount(item.play_count || 0) }}
                </span>
                <span v-if="item.type === 'article'" class="text-xs text-ink-muted ml-auto">
                  阅读 {{ formatCount(item.view_count || 0) }}
                </span>
              </div>
            </div>
          </article>
        </div>

        <!-- 空状态 -->
        <EmptyState v-else-if="!loading" text="暂无动态，关注更多 UP 主或发布稿件后会出现在这里" />

        <!-- 分页 -->
        <Pagination
          v-if="filteredList.length"
          :current="page"
          :total="total"
          :page-size="pageSize"
          @change="onPageChange"
        />
      </main>
    </div>
  </div>
</template>

<style scoped>
/* B站风格三栏布局：左 270px / 中 flex / 右 290px（这里隐藏右侧） */
.dyn-grid {
  display: flex;
  align-items: flex-start;
  gap: 22px;
}

.dyn-left {
  flex: 0 0 240px;
  width: 240px;
}

.dyn-center {
  flex: 1 1 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

/* Tab 栏：白底圆角带边框 */
.dyn-tabs {
  display: flex;
  align-items: center;
  gap: 28px;
  padding: 0 20px;
  min-height: 48px;
  background: #fff;
  border: 1px solid #e3e5e7;
  border-radius: 8px;
  box-sizing: border-box;
}

/* 响应式：窄屏隐藏左侧栏 */
@media (max-width: 1024px) {
  .dyn-left {
    display: none;
  }
}
</style>
