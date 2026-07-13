<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import Artplayer from 'artplayer'
import ArtplayerPluginDanmuku from 'artplayer-plugin-danmuku'
import type { DanmakuItem } from '@/types'
import { sendDanmaku } from '@/api/interaction'
import { useUserStore } from '@/stores/user'

const props = defineProps<{
  url: string
  coverUrl: string
  danmakuList: DanmakuItem[]
  videoId: number
  /** 从历史记录恢复的播放进度（秒），用于回看定位 */
  resumeTime?: number
}>()

const emit = defineEmits<{
  (e: 'danmaku-sent'): void
}>()

const container = ref<HTMLDivElement | null>(null)
const errorMsg = ref('')
const danmakuInput = ref('')
const sendingDanmaku = ref(false)
const userStore = useUserStore()
let art: Artplayer | null = null
let lastTime = 0

// 弹幕样式选项
const danmakuColor = ref('#ffffff')
const danmakuMode = ref<0 | 1 | 2>(0) // 0=滚动, 1=顶部, 2=底部
const showColorPicker = ref(false)
const presetColors = [
  '#ffffff', '#fe0302', '#ff7204', '#ffcc02',
  '#1ee934', '#00c6ea', '#0099ff', '#3456ee',
  '#a32dcc', '#ff6699', '#999999', '#000000',
]

const modeLabels: Record<number, string> = { 0: '滚动', 1: '顶部', 2: '底部' }

function getCurrentTime(): number {
  return art?.currentTime ?? lastTime
}

// 将 DanmakuItem[] 转换为 Artplayer 弹幕插件所需的 Danmu[] 格式
function toDanmuList(list: DanmakuItem[]) {
  return list.map((d) => ({
    text: d.content,
    time: d.video_time,
    color: d.color || '#fff',
    mode: (d.mode ?? 0) as 0 | 1 | 2,
  }))
}

// 获取弹幕插件实例
// artplayer-plugin-danmuku 插件通过 name='artplayerPluginDanmuku' 挂载到 art.plugins
function getDanmakuPlugin(): any | null {
  if (!art) return null
  const plugin = (art as any).plugins?.artplayerPluginDanmuku
  return plugin || null
}

// 实时插入一条弹幕到播放器（不重新加载整个列表）
// 关键：不传 time 参数，让插件自动使用 currentTime + 0.5
// artplayer-plugin-danmuku 的 emit() 将弹幕加入 wait 队列，
// update() 只在 video.playing 时处理，readys 只选取 currentTime±0.1s 范围的弹幕。
// 如果传入精确的 currentTime，视频暂停后恢复播放时时间偏移 >0.1s 就会错过弹幕。
// 使用 currentTime + 0.5（与插件内置发射器行为一致）给 0.5s 缓冲，确保弹幕能被正确选取。
function pushDanmakuToLocal(item: { text: string; color: string; mode: 0 | 1 | 2 }) {
  const plugin = getDanmakuPlugin()
  if (plugin && typeof plugin.emit === 'function') {
    plugin.emit({
      text: item.text,
      color: item.color || '#fff',
      mode: item.mode,
      border: true, // 描边，与插件内置发射器一致，区分用户发送的弹幕
    })
  }
}

// 将弹幕列表全量重载到播放器
// 关键：reset() 不会清空 queue，load(数组) 只追加不清空
// 正确做法：用 config() 更新 option.danmuku，然后 load() 不传参数
// load() 不传参数时才会真正清空 queue 并重新加载 option.danmuku
async function reloadDanmakuList(list: DanmakuItem[]) {
  const plugin = getDanmakuPlugin()
  if (!plugin) return
  const danmuList = toDanmuList(list)
  // 更新插件内部的弹幕数据源
  if (typeof plugin.config === 'function') {
    plugin.config({ danmuku: danmuList })
  }
  // load() 不传参数：清空 queue + states + DOM，然后从 option.danmuku 重新加载
  if (typeof plugin.load === 'function') {
    try {
      await plugin.load()
    } catch {
      // 弹幕加载失败不影响视频播放
    }
  }
}

// 供父组件直接调用的弹幕加载方法（不依赖 watch 触发）
function loadDanmakuList(list: DanmakuItem[]) {
  reloadDanmakuList(list)
}

async function handleSendDanmaku() {
  const content = danmakuInput.value.trim()
  if (!content || sendingDanmaku.value) return
  if (!userStore.isLogin) return
  sendingDanmaku.value = true
  const currentTime = art?.currentTime ?? 0
  const color = danmakuColor.value
  const mode = danmakuMode.value
  try {
    await sendDanmaku({
      video_id: props.videoId,
      content,
      video_time: currentTime,
      color,
      font_size: '25',
      mode,
    })
    // 本地立即插入弹幕到播放器（即时反馈，无需重新加载整个列表）
    // 注意：传给后端的 video_time 是精确的 currentTime（用于持久化），
    // 但 pushDanmakuToLocal 不传 time，让插件使用 currentTime + 0.5（用于即时显示）
    pushDanmakuToLocal({ text: content, color, mode })
    danmakuInput.value = ''
    // 通知父组件弹幕数 +1
    // 注意：父组件不应重新加载弹幕列表，否则会清空刚 emit 的弹幕
    emit('danmaku-sent')
  } catch {
    // 错误已由拦截器提示
  } finally {
    sendingDanmaku.value = false
  }
}

// 点击外部关闭颜色选择器
function closeColorPicker(e: MouseEvent) {
  const target = e.target as HTMLElement
  if (!target.closest('.color-picker-area')) {
    showColorPicker.value = false
  }
}

onMounted(() => {
  if (!container.value) return
  if (!props.url) {
    errorMsg.value = '视频地址无效，可能仍在转码中，请稍后刷新重试'
    return
  }
  art = new Artplayer({
    container: container.value,
    url: props.url,
    poster: props.coverUrl,
    volume: 0.7,
    autoplay: false,
    pip: true,
    setting: true,
    playbackRate: true,
    fullscreen: true,
    fullscreenWeb: true,
    plugins: [
      ArtplayerPluginDanmuku({
        danmuku: toDanmuList(props.danmakuList),
        // 禁用插件自带弹幕发射器，使用 VideoPlayer.vue 自定义输入框调用后端 API
        emitter: false,
      }),
    ],
  })

  // 从历史记录恢复播放进度
  if (props.resumeTime && props.resumeTime > 0) {
    art.on('ready', () => {
      if (art) {
        art.currentTime = props.resumeTime
      }
    })
  }

  art.on('error', () => {
    errorMsg.value = '视频加载失败，可能仍在转码中，请稍后刷新重试'
  })

  document.addEventListener('click', closeColorPicker)
})

// 监听 danmakuList 变化：父组件异步加载弹幕完成后同步到播放器
// 同时也通过 defineExpose 暴露 loadDanmakuList 方法供父组件直接调用
watch(
  () => props.danmakuList,
  (newList) => {
    if (art) {
      reloadDanmakuList(newList)
    }
  }
)

// 监听 videoId 变化：路由切换到新视频时清空播放器弹幕
watch(
  () => props.videoId,
  () => {
    const plugin = getDanmakuPlugin()
    if (plugin) {
      // 清空弹幕数据源，再 load() 不传参数清空 queue
      if (typeof plugin.config === 'function') {
        plugin.config({ danmuku: [] })
      }
      if (typeof plugin.load === 'function') {
        plugin.load()
      }
    }
  }
)

onUnmounted(() => {
  document.removeEventListener('click', closeColorPicker)
  if (art) {
    lastTime = art.currentTime
    art.destroy(false)
    art = null
  }
})

defineExpose({ getCurrentTime, loadDanmakuList })
</script>

<template>
  <div>
    <div class="w-full aspect-video bg-black rounded-card overflow-hidden relative">
      <div ref="container" class="w-full aspect-video" />
      <!-- 错误提示 -->
      <div
        v-if="errorMsg"
        class="absolute inset-0 flex items-center justify-center text-white/80 text-sm bg-black/90"
      >
        {{ errorMsg }}
      </div>
    </div>
    <!-- 弹幕发送区（B站风格：模式选择 + 颜色选择 + 输入框 + 发送） -->
    <div v-if="userStore.isLogin" class="mt-2 flex items-center gap-2 flex-wrap">
      <!-- 弹幕模式选择 -->
      <div class="flex items-center bg-surface-subtle rounded-full h-9 px-1 shrink-0">
        <button
          v-for="m in [0, 1, 2]"
          :key="m"
          class="px-2.5 h-7 rounded-full text-xs font-medium transition"
          :class="danmakuMode === m ? 'bg-secondary text-white' : 'text-ink-muted hover:text-ink'"
          :title="modeLabels[m]"
          @click="danmakuMode = m as 0 | 1 | 2"
        >
          {{ modeLabels[m] }}
        </button>
      </div>

      <!-- 颜色选择 -->
      <div class="relative color-picker-area shrink-0">
        <button
          class="w-9 h-9 rounded-full bg-surface-subtle flex items-center justify-center text-sm font-bold border-2 transition hover:bg-surface-muted"
          :style="{ color: danmakuColor, borderColor: danmakuColor !== '#ffffff' ? danmakuColor : 'transparent' }"
          title="选择颜色"
          @click.stop="showColorPicker = !showColorPicker"
        >
          A
        </button>
        <!-- 颜色选择面板 -->
        <div
          v-if="showColorPicker"
          class="absolute top-10 left-0 z-20 bg-white rounded-lg shadow-lg border border-surface-muted p-2 grid grid-cols-6 gap-1.5"
        >
          <button
            v-for="color in presetColors"
            :key="color"
            class="w-6 h-6 rounded-full border-2 transition hover:scale-110"
            :class="danmakuColor === color ? 'border-primary' : 'border-transparent'"
            :style="{ background: color }"
            @click="danmakuColor = color; showColorPicker = false"
          />
        </div>
      </div>

      <!-- 输入框 -->
      <input
        v-model="danmakuInput"
        type="text"
        placeholder="发个弹幕见证当下"
        maxlength="100"
        class="flex-1 min-w-0 h-9 px-4 bg-surface-subtle rounded-full text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
        @keydown.enter="handleSendDanmaku"
      />

      <!-- 发送按钮 -->
      <button
        class="h-9 px-5 rounded-full bg-secondary text-white text-sm font-medium hover:bg-secondary-dark transition disabled:opacity-50 shrink-0"
        :disabled="!danmakuInput.trim() || sendingDanmaku"
        @click="handleSendDanmaku"
      >
        发送
      </button>
    </div>
  </div>
</template>
