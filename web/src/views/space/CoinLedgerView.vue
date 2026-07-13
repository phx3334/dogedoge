<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { Coins, ArrowUpCircle, ArrowDownCircle } from 'lucide-vue-next'
import { getCoinLedger } from '@/api/coin'
import { useUserStore } from '@/stores/user'
import type { CoinLedgerItem, PaginatedResp } from '@/types'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'

const userStore = useUserStore()
const list = ref<CoinLedgerItem[]>([])
const page = ref(1)
const total = ref(0)
const pageSize = 15
const loading = ref(false)
const reasonType = ref('')

const filters = [
  { value: '', label: '全部' },
  { value: 'coin_video', label: '投币' },
  { value: 'coin_income', label: '收入' },
  { value: 'daily_login', label: '登录奖励' },
  { value: 'comment_reward', label: '评论奖励' },
]

const balance = computed(() => {
  const tenths = userStore.userInfo?.coin_balance_tenths ?? 0
  return (tenths / 10).toFixed(1)
})

function fmtDate(d: string) {
  try {
    return new Date(d).toLocaleDateString()
  } catch {
    return d
  }
}

function coinValue(item: CoinLedgerItem) {
  return (item.delta_tenths / 10).toFixed(1)
}

function isPositive(item: CoinLedgerItem) {
  return item.delta_tenths > 0
}

async function load() {
  loading.value = true
  try {
    const res: PaginatedResp<CoinLedgerItem> = await getCoinLedger(reasonType.value, page.value, pageSize)
    list.value = res.list || []
    total.value = res.total || 0
  } finally {
    loading.value = false
  }
}

async function onFilterChange() {
  page.value = 1
  await load()
}

async function onPageChange(p: number) {
  page.value = p
  await load()
  window.scrollTo({ top: 0 })
}

watch(reasonType, () => onFilterChange())

onMounted(() => {
  load()
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-bold text-ink">硬币流水</h2>
      <!-- 余额 -->
      <div class="flex items-center gap-1.5 px-3 h-9 bg-secondary/10 text-secondary rounded-card text-sm">
        <Coins :size="16" />
        <span>余额：{{ balance }}</span>
      </div>
    </div>

    <!-- 筛选 -->
    <div class="flex items-center gap-2 bg-white rounded-card shadow-card p-3">
      <span class="text-sm text-ink-secondary">类型：</span>
      <select
        v-model="reasonType"
        class="h-8 px-3 bg-surface-subtle rounded text-sm text-ink focus:outline-none focus:ring-2 focus:ring-primary/30 cursor-pointer"
      >
        <option v-for="f in filters" :key="f.value" :value="f.value">{{ f.label }}</option>
      </select>
    </div>

    <!-- 流水列表 -->
    <div class="bg-white rounded-card shadow-card p-4">
      <div v-if="loading" class="py-12 text-center text-sm text-ink-muted">加载中...</div>
      <template v-else-if="list.length">
        <div class="space-y-2">
          <div
            v-for="item in list"
            :key="item.id"
            class="flex items-center gap-3 p-3 rounded hover:bg-surface-subtle transition"
          >
            <!-- 图标 -->
            <div
              class="w-9 h-9 rounded-full flex items-center justify-center shrink-0"
              :class="isPositive(item) ? 'bg-green-50 text-green-500' : 'bg-red-50 text-red-500'"
            >
              <component :is="isPositive(item) ? ArrowDownCircle : ArrowUpCircle" :size="18" />
            </div>
            <!-- 内容 -->
            <div class="flex-1 min-w-0">
              <p class="text-sm text-ink truncate">{{ item.reason }}</p>
              <p class="text-xs text-ink-muted mt-0.5">{{ fmtDate(item.created_at) }}</p>
            </div>
            <!-- 变动量 -->
            <div
              class="flex items-center gap-1 text-sm font-medium shrink-0"
              :class="isPositive(item) ? 'text-green-500' : 'text-red-500'"
            >
              <Coins :size="14" />
              <span>{{ isPositive(item) ? '+' : '' }}{{ coinValue(item) }}</span>
            </div>
          </div>
        </div>
        <Pagination :current="page" :total="total" :page-size="pageSize" @change="onPageChange" />
      </template>
      <EmptyState v-else text="暂无硬币流水" />
    </div>
  </div>
</template>
