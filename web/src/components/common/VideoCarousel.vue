<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRouter } from 'vue-router'
import type { HomeVideoInfo } from '@/types'
import { Play as PlayIcon } from 'lucide-vue-next'

const props = defineProps<{
  videos: HomeVideoInfo[]
  interval?: number
}>()

const router = useRouter()
const current = ref(0)
let timer: ReturnType<typeof setInterval> | null = null

const count = computed(() => props.videos.length)
const currentVideo = computed(() => (count.value ? props.videos[current.value] : null))

function goTo(i: number) {
  if (count.value === 0) return
  current.value = (i + count.value) % count.value
}
function next() {
  goTo(current.value + 1)
}
function prev() {
  goTo(current.value - 1)
}

function startAuto() {
  stopAuto()
  if (count.value <= 1) return
  const ms = props.interval ?? 4000
  timer = setInterval(next, ms)
}
function stopAuto() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

function openVideo(id: number) {
  router.push(`/video/${id}`)
}

function formatCount(n: number): string {
  if (n >= 10000) return (n / 10000).toFixed(1) + '万'
  return String(n)
}

onMounted(startAuto)
onBeforeUnmount(stopAuto)
watch(count, startAuto)
</script>

<template>
  <div
    class="relative w-full h-full min-h-0 rounded-card overflow-hidden bg-surface-muted group flex flex-col"
    @mouseenter="stopAuto"
    @mouseleave="startAuto"
  >
    <template v-if="count > 0">
      <!-- 图片区 -->
      <div class="relative flex-1 min-h-0 overflow-hidden">
        <div
          v-for="(video, idx) in videos"
          :key="video.id"
          class="absolute inset-0 cursor-pointer transition-opacity duration-500"
          :class="idx === current ? 'opacity-100 z-10' : 'opacity-0 z-0'"
          @click="openVideo(video.id)"
        >
          <img :src="video.cover_url" :alt="video.title" class="w-full h-full object-cover" />
          <div class="absolute inset-0 bg-gradient-to-t from-black/50 to-transparent" />
        </div>

        <!-- 热度角标 -->
        <span class="absolute top-3 left-3 z-20 flex items-center gap-1 px-2 py-1 rounded-full bg-black/50 text-white text-xs font-medium">
          <PlayIcon :size="12" /> 热度 TOP {{ current + 1 }}
        </span>

        <!-- 左右箭头 -->
        <button
          class="absolute left-2 top-1/2 -translate-y-1/2 w-9 h-9 z-20 flex items-center justify-center rounded-full bg-black/40 text-white opacity-0 group-hover:opacity-100 transition hover:bg-black/60"
          @click.stop="prev"
        >
          <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 18l-6-6 6-6"/></svg>
        </button>
        <button
          class="absolute right-2 top-1/2 -translate-y-1/2 w-9 h-9 z-20 flex items-center justify-center rounded-full bg-black/40 text-white opacity-0 group-hover:opacity-100 transition hover:bg-black/60"
          @click.stop="next"
        >
          <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6"/></svg>
        </button>

        <!-- 指示点 -->
        <div class="absolute bottom-3 right-4 z-20 flex items-center gap-1.5">
          <button
            v-for="(v, i) in videos"
            :key="'dot-' + v.id"
            class="h-2 rounded-full transition-all"
            :class="i === current ? 'w-5 bg-white' : 'w-2 bg-white/50 hover:bg-white/80'"
            @click.stop="goTo(i)"
          />
        </div>
      </div>

      <!-- 视频名称（轮播图下方） -->
      <div
        class="shrink-0 bg-white px-3 py-2 cursor-pointer hover:bg-surface-subtle transition"
        @click="currentVideo && openVideo(currentVideo.id)"
      >
        <p class="text-sm font-medium text-ink leading-5 truncate">{{ currentVideo?.title }}</p>
        <p class="mt-0.5 text-xs text-ink-muted truncate">{{ currentVideo?.up_name }} · 播放 {{ formatCount(currentVideo?.play_count ?? 0) }}</p>
      </div>
    </template>

    <!-- 空状态 -->
    <div v-else class="w-full h-full flex items-center justify-center text-sm text-ink-muted">
      暂无热门视频
    </div>
  </div>
</template>
