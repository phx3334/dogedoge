<script setup lang="ts">
import { ref } from 'vue'
import { useUserStore } from '@/stores/user'
import VideoUpload from '@/views/upload/VideoUpload.vue'
import ArticleUpload from '@/views/upload/ArticleUpload.vue'

const userStore = useUserStore()
const tab = ref<'video' | 'article'>('video')
</script>

<template>
  <div class="max-w-[860px] mx-auto px-4 py-6">
    <h1 class="text-2xl font-bold text-ink mb-1">投稿中心</h1>
    <p v-if="userStore.userInfo" class="text-sm text-ink-muted mb-4">
      Hi, {{ userStore.userInfo.username }}，开始你的创作吧
    </p>

    <!-- Tab 切换 -->
    <div class="flex gap-1 border-b border-surface-muted mb-6">
      <button
        class="px-4 py-2 text-sm font-medium transition relative"
        :class="tab === 'video' ? 'text-primary' : 'text-ink-secondary hover:text-ink'"
        @click="tab = 'video'"
      >
        视频投稿
        <span v-if="tab === 'video'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-primary" />
      </button>
      <button
        class="px-4 py-2 text-sm font-medium transition relative"
        :class="tab === 'article' ? 'text-primary' : 'text-ink-secondary hover:text-ink'"
        @click="tab = 'article'"
      >
        文章投稿
        <span v-if="tab === 'article'" class="absolute bottom-0 left-0 right-0 h-0.5 bg-primary" />
      </button>
    </div>

    <VideoUpload v-if="tab === 'video'" />
    <ArticleUpload v-else />
  </div>
</template>
