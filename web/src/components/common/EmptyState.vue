<script setup lang="ts">
defineProps<{
  text?: string
}>()
</script>

<template>
  <div class="flex flex-col items-center justify-center py-16 text-ink-muted">
    <div class="text-sm">{{ text || '暂无数据' }}</div>
  </div>
</template>
