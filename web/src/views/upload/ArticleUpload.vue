<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { saveArticleDraft, publishArticle } from '@/api/article'
import { Send, ImagePlus, X } from 'lucide-vue-next'

const router = useRouter()

const articleTitle = ref('')
const articleContent = ref('')
const articleTags = ref('')
const articlePublishing = ref(false)
const images = ref<string[]>([])
const fileInput = ref<HTMLInputElement | null>(null)

const articleValid = computed(
  () => !!articleTitle.value.trim() && !!articleContent.value.trim()
)

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

async function handlePublishArticle() {
  if (!articleValid.value || articlePublishing.value) return
  articlePublishing.value = true
  try {
    // 1. 先保存草稿，获取 article_id
    const draftRes = await saveArticleDraft({
      title: articleTitle.value.trim(),
      body_md: articleContent.value.trim(),
      tags: articleTags.value.split(',').map((t) => t.trim()).filter(Boolean),
      images: images.value,
    })
    // 2. 用 article_id 发布
    await publishArticle(draftRes.article_id)
    router.push(`/article/${draftRes.article_id}`)
  } catch (err: any) {
    alert(err?.message || '发布失败')
  } finally {
    articlePublishing.value = false
  }
}
</script>

<template>
  <div class="space-y-4">
    <div>
      <label class="block text-sm font-medium text-ink mb-2">
        标题 <span class="text-primary">*</span>
      </label>
      <input
        v-model="articleTitle"
        type="text"
        placeholder="请输入文章标题"
        class="w-full h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
      />
    </div>

    <div>
      <label class="block text-sm font-medium text-ink mb-2">
        正文 <span class="text-primary">*</span>
      </label>
      <textarea
        v-model="articleContent"
        rows="12"
        placeholder="请输入文章正文..."
        class="w-full p-4 bg-surface-subtle rounded-lg text-sm resize-y focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
      />
    </div>

    <!-- 图片上传 -->
    <div>
      <label class="block text-sm font-medium text-ink mb-2">图片（最多9张）</label>
      <div v-if="images.length" class="flex flex-wrap gap-2 mb-2">
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
      <button
        v-if="images.length < 9"
        type="button"
        class="flex items-center gap-1.5 px-4 h-10 bg-surface-subtle text-ink-secondary rounded-lg text-sm hover:bg-surface-muted transition"
        @click="fileInput?.click()"
      >
        <ImagePlus :size="16" />
        上传图片
      </button>
      <input
        ref="fileInput"
        type="file"
        multiple
        accept="image/*"
        class="hidden"
        @change="handleFileSelect"
      />
    </div>

    <div>
      <label class="block text-sm font-medium text-ink mb-2">标签</label>
      <input
        v-model="articleTags"
        type="text"
        placeholder="多个标签用英文逗号分隔"
        class="w-full h-11 px-4 bg-surface-subtle rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 transition"
      />
    </div>

    <button
      :disabled="!articleValid || articlePublishing"
      class="px-6 h-11 bg-primary text-white rounded-lg text-sm font-medium hover:bg-primary-dark transition disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-1"
      @click="handlePublishArticle"
    >
      <Send :size="16" />
      {{ articlePublishing ? '发布中...' : '发布文章' }}
    </button>
  </div>
</template>
