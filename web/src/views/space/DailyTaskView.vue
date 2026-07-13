<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { CheckCircle2, Circle, LogIn, Eye, Star, Coins, MessageSquare } from 'lucide-vue-next'
import { getUserLevel } from '@/api/user'
import { getDailyTask } from '@/api/daily'
import type { UserLevelResp, DailyTaskResp } from '@/types'
import EmptyState from '@/components/common/EmptyState.vue'

const level = ref<UserLevelResp | null>(null)
const task = ref<DailyTaskResp | null>(null)
const todayExp = ref(0)
const loading = ref(false)

const expProgress = computed(() => {
  if (!level.value) return 0
  const cur = level.value.current_level_exp
  const next = level.value.next_level_exp
  if (!next) return 100 // 满级
  const exp = level.value.experience
  const done = exp - cur
  const total = next - cur
  if (total <= 0) return 100
  return Math.min(100, Math.max(0, Math.round((done / total) * 100)))
})

const rules = [
  { icon: LogIn, label: '每日登录', exp: '+10 经验' },
  { icon: Coins, label: '投币', exp: '+20 经验/枚' },
  { icon: MessageSquare, label: '评论', exp: '+5 经验/次' },
]

// 等级阈值
const levelThresholds = [
  { level: 1, exp: 50 },
  { level: 2, exp: 200 },
  { level: 3, exp: 500 },
  { level: 4, exp: 1000 },
  { level: 5, exp: 2500 },
  { level: 6, exp: 5000 },
]

async function load() {
  loading.value = true
  try {
    const [lv, tk] = await Promise.all([getUserLevel(), getDailyTask()])
    level.value = lv
    task.value = tk
    todayExp.value = tk.today_exp ?? 0
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  load()
})
</script>

<template>
  <div class="space-y-4">
    <h2 class="text-lg font-bold text-ink">每日任务</h2>

    <div v-if="loading" class="py-12 text-center text-sm text-ink-muted">加载中...</div>

    <template v-else>
      <!-- 等级卡片 -->
      <div v-if="level" class="bg-white rounded-card shadow-card p-5">
        <div class="flex items-center gap-4">
          <!-- 等级徽章 -->
          <div class="w-16 h-16 rounded-full bg-gradient-to-br from-primary to-primary-dark flex items-center justify-center shrink-0">
            <div class="text-center">
              <div class="text-xs text-white/80">Lv</div>
              <div class="text-2xl font-bold text-white leading-none">{{ level.level }}</div>
            </div>
          </div>
          <!-- 等级信息 -->
          <div class="flex-1 min-w-0">
            <div class="flex items-center justify-between mb-1">
              <span class="text-sm text-ink">当前等级 Lv {{ level.level }}</span>
              <span class="text-xs text-ink-muted">{{ level.experience - level.current_level_exp }} / {{ level.next_level_exp - level.current_level_exp }} 经验</span>
            </div>
            <!-- 进度条 -->
            <div class="h-2 bg-surface-muted rounded-full overflow-hidden">
              <div
                class="h-full bg-primary rounded-full transition-all duration-500"
                :style="{ width: expProgress + '%' }"
              ></div>
            </div>
            <p class="text-xs text-ink-muted mt-1">距离下一级还需 {{ Math.max(0, level.next_level_exp - level.experience) }} 经验</p>
          </div>
        </div>
      </div>

      <!-- 今日任务 -->
      <div v-if="task" class="bg-white rounded-card shadow-card p-4">
        <div class="flex items-center justify-between mb-3">
          <h3 class="text-base font-medium text-ink">今日任务</h3>
          <div class="flex items-center gap-1 text-sm text-primary">
            <Star :size="14" />
            <span>今日已获 {{ todayExp }} 经验</span>
          </div>
        </div>

        <div class="space-y-2">
          <!-- 登录奖励 -->
          <div class="flex items-center gap-3 p-3 rounded bg-surface-subtle">
            <div class="w-9 h-9 rounded-full bg-primary/10 text-primary flex items-center justify-center shrink-0">
              <LogIn :size="18" />
            </div>
            <div class="flex-1">
              <div class="text-sm text-ink">每日登录</div>
              <div class="text-xs text-ink-muted">+10 经验</div>
            </div>
            <div class="flex items-center gap-1 text-sm" :class="task.login_done ? 'text-green-500' : 'text-ink-muted'">
              <component :is="task.login_done ? CheckCircle2 : Circle" :size="18" />
              <span>{{ task.login_done ? '已完成' : '未完成' }}</span>
            </div>
          </div>

          <!-- 观看视频 -->
          <div class="flex items-center gap-3 p-3 rounded bg-surface-subtle">
            <div class="w-9 h-9 rounded-full bg-secondary/10 text-secondary flex items-center justify-center shrink-0">
              <Eye :size="18" />
            </div>
            <div class="flex-1">
              <div class="text-sm text-ink">观看视频</div>
              <div class="text-xs text-ink-muted">观看视频获取经验</div>
            </div>
            <div class="flex items-center gap-1 text-sm" :class="task.watch_done ? 'text-green-500' : 'text-ink-muted'">
              <component :is="task.watch_done ? CheckCircle2 : Circle" :size="18" />
              <span>{{ task.watch_done ? '已完成' : '未完成' }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 经验规则说明 -->
      <div class="bg-white rounded-card shadow-card p-4">
        <h3 class="text-base font-medium text-ink mb-3">经验规则说明</h3>
        <div class="space-y-2 mb-4">
          <div
            v-for="rule in rules"
            :key="rule.label"
            class="flex items-center gap-3 p-2 rounded hover:bg-surface-subtle transition"
          >
            <div class="w-8 h-8 rounded-full bg-surface-subtle flex items-center justify-center shrink-0 text-ink-secondary">
              <component :is="rule.icon" :size="16" />
            </div>
            <span class="flex-1 text-sm text-ink">{{ rule.label }}</span>
            <span class="text-sm text-primary">{{ rule.exp }}</span>
          </div>
        </div>

        <!-- 等级阈值 -->
        <div class="border-t border-surface-muted pt-3">
          <div class="text-xs text-ink-muted mb-2">等级阈值</div>
          <div class="grid grid-cols-3 sm:grid-cols-6 gap-2">
            <div
              v-for="th in levelThresholds"
              :key="th.level"
              class="text-center p-2 rounded"
              :class="level && level.level >= th.level ? 'bg-primary/10 text-primary' : 'bg-surface-subtle text-ink-muted'"
            >
              <div class="text-xs">Lv {{ th.level }}</div>
              <div class="text-xs font-medium mt-0.5">{{ th.exp }}</div>
            </div>
          </div>
        </div>
      </div>
    </template>

    <EmptyState v-if="!loading && !level && !task" text="暂无任务数据" />
  </div>
</template>
