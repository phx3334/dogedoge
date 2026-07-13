<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { Plus, Folder, Trash2, Pencil, Check, X, FolderInput } from 'lucide-vue-next'
import {
  getFavoriteFolders,
  createFavoriteFolder,
  updateFavoriteFolder,
  deleteFavoriteFolder,
  getFolderVideos,
  moveFavorite,
} from '@/api/favorite'
import { uploadImage } from '@/api/user'
import { unfavoriteVideo } from '@/api/interaction'
import type { FavoriteFolder, HomeVideoInfo, PaginatedResp } from '@/types'
import VideoCard from '@/components/common/VideoCard.vue'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'

const folders = ref<FavoriteFolder[]>([])
const selectedId = ref<number | null>(null)
const videos = ref<HomeVideoInfo[]>([])
const page = ref(1)
const total = ref(0)
const pageSize = 12
const loadingFolders = ref(false)
const loadingVideos = ref(false)

// 新建收藏夹
const showCreate = ref(false)
const newTitle = ref('')
const newCoverFile = ref<File | null>(null)
const newCoverPreview = ref('')
const createCoverInput = ref<HTMLInputElement | null>(null)

// 编辑收藏夹
const editingId = ref<number | null>(null)
const editingTitle = ref('')
const editCoverFile = ref<File | null>(null)
const editCoverPreview = ref('')
const editCoverInput = ref<HTMLInputElement | null>(null)

// 移动收藏弹窗
const moveTargetVideo = ref<HomeVideoInfo | null>(null)
const moveDestFolderId = ref<number | null>(null)
const moving = ref(false)
const removing = ref<number | null>(null)

const selectedFolder = computed(() => folders.value.find(f => f.id === selectedId.value))
const movableFolders = computed(() => folders.value.filter(f => f.id !== selectedId.value))

async function loadFolders() {
  loadingFolders.value = true
  try {
    folders.value = await getFavoriteFolders()
    if (folders.value.length && selectedId.value === null) {
      selectedId.value = folders.value[0].id
      await loadVideos()
    } else if (selectedId.value !== null && !folders.value.find(f => f.id === selectedId.value)) {
      selectedId.value = folders.value[0]?.id ?? null
      await loadVideos()
    }
  } finally {
    loadingFolders.value = false
  }
}

async function loadVideos() {
  if (selectedId.value === null) {
    videos.value = []
    total.value = 0
    return
  }
  loadingVideos.value = true
  try {
    const res: PaginatedResp<HomeVideoInfo> = await getFolderVideos(selectedId.value, page.value, pageSize)
    videos.value = res.list || []
    total.value = res.total || 0
  } finally {
    loadingVideos.value = false
  }
}

async function selectFolder(id: number) {
  if (selectedId.value === id) return
  selectedId.value = id
  page.value = 1
  await loadVideos()
}

async function handleCreate() {
  const title = newTitle.value.trim()
  if (!title) return
  let coverUrl = ''
  if (newCoverFile.value) {
    try {
      const res = await uploadImage(newCoverFile.value)
      coverUrl = res.url
    } catch {
      // 封面上传失败不阻止创建
    }
  }
  await createFavoriteFolder(title, coverUrl || undefined)
  newTitle.value = ''
  newCoverFile.value = null
  newCoverPreview.value = ''
  showCreate.value = false
  await loadFolders()
}

function handleNewCoverChange(e: Event) {
  const target = e.target as HTMLInputElement
  if (target.files && target.files[0]) {
    newCoverFile.value = target.files[0]
    const reader = new FileReader()
    reader.onload = () => {
      newCoverPreview.value = reader.result as string
    }
    reader.readAsDataURL(target.files[0])
  }
}

function clearNewCover() {
  newCoverFile.value = null
  newCoverPreview.value = ''
}

function handleEditCoverChange(e: Event) {
  const target = e.target as HTMLInputElement
  if (target.files && target.files[0]) {
    editCoverFile.value = target.files[0]
    const reader = new FileReader()
    reader.onload = () => {
      editCoverPreview.value = reader.result as string
    }
    reader.readAsDataURL(target.files[0])
  }
}

function clearEditCover() {
  editCoverFile.value = null
  editCoverPreview.value = ''
}

function startEdit(folder: FavoriteFolder) {
  editingId.value = folder.id
  editingTitle.value = folder.title
}

function cancelEdit() {
  editingId.value = null
  editingTitle.value = ''
  editCoverFile.value = null
  editCoverPreview.value = ''
}

async function saveEdit() {
  const title = editingTitle.value.trim()
  if (!title || editingId.value === null) return
  let coverUrl = ''
  if (editCoverFile.value) {
    try {
      const res = await uploadImage(editCoverFile.value)
      coverUrl = res.url
    } catch {
      // 封面上传失败不阻止更新
    }
  }
  await updateFavoriteFolder(editingId.value, title, coverUrl || undefined)
  cancelEdit()
  await loadFolders()
}

async function handleDelete(folder: FavoriteFolder) {
  if (folder.is_default) return
  if (!confirm(`确认删除收藏夹「${folder.title}」？`)) return
  await deleteFavoriteFolder(folder.id)
  if (selectedId.value === folder.id) {
    selectedId.value = null
    page.value = 1
  }
  await loadFolders()
}

function openMove(video: HomeVideoInfo) {
  moveTargetVideo.value = video
  moveDestFolderId.value = movableFolders.value[0]?.id ?? null
}

function cancelMove() {
  moveTargetVideo.value = null
  moveDestFolderId.value = null
}

async function confirmMove() {
  if (!moveTargetVideo.value || moveDestFolderId.value === null) return
  moving.value = true
  try {
    await moveFavorite(moveTargetVideo.value.id, moveDestFolderId.value)
    cancelMove()
    await loadVideos()
  } finally {
    moving.value = false
  }
}

async function handleRemove(video: HomeVideoInfo) {
  if (!confirm(`确认从「${selectedFolder.value?.title}」移除「${video.title}」？`)) return
  removing.value = video.id
  try {
    await unfavoriteVideo(video.id, selectedId.value!)
    await loadVideos()
    await loadFolders()
  } finally {
    removing.value = null
  }
}

async function onPageChange(p: number) {
  page.value = p
  await loadVideos()
  window.scrollTo({ top: 0 })
}

onMounted(() => {
  loadFolders()
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-bold text-ink">我的收藏</h2>
      <button
        class="flex items-center gap-1 px-3 h-9 rounded-card bg-primary text-white text-sm hover:bg-primary-dark transition"
        @click="showCreate = !showCreate"
      >
        <Plus :size="16" />
        新建收藏夹
      </button>
    </div>

    <!-- 新建输入框 -->
    <div v-if="showCreate" class="bg-white rounded-card shadow-card p-3">
      <div class="flex items-center gap-2">
        <input
          v-model="newTitle"
          type="text"
          placeholder="收藏夹标题"
          maxlength="30"
          class="flex-1 h-9 px-3 bg-surface-subtle rounded text-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
          @keyup.enter="handleCreate"
        />
        <button class="w-9 h-9 flex items-center justify-center rounded bg-primary text-white hover:bg-primary-dark" @click="handleCreate">
          <Check :size="16" />
        </button>
        <button class="w-9 h-9 flex items-center justify-center rounded bg-surface-muted text-ink-secondary hover:bg-surface-muted/70" @click="showCreate = false">
          <X :size="16" />
        </button>
      </div>
      <!-- 封面上传 -->
      <div class="mt-2">
        <div v-if="!newCoverPreview" class="flex items-center gap-2">
          <span class="text-xs text-ink-muted">封面（可选）：</span>
          <button class="text-xs text-primary hover:underline" @click="createCoverInput?.click()">上传图片</button>
          <input ref="createCoverInput" type="file" accept="image/*" class="hidden" @change="handleNewCoverChange" />
        </div>
        <div v-else class="relative inline-block">
          <img :src="newCoverPreview" alt="封面预览" class="w-16 h-12 object-cover rounded" />
          <button class="absolute -top-1.5 -right-1.5 w-4 h-4 bg-black/60 rounded-full text-white flex items-center justify-center text-[10px]" @click="clearNewCover">×</button>
        </div>
      </div>
    </div>

    <div class="flex gap-4">
      <!-- 收藏夹列表 -->
      <div class="w-60 shrink-0 space-y-2">
        <div v-if="loadingFolders" class="text-sm text-ink-muted py-4 text-center">加载中...</div>
        <div
          v-for="folder in folders"
          :key="folder.id"
          class="group bg-white rounded-card shadow-card p-3 cursor-pointer transition"
          :class="selectedId === folder.id ? 'ring-2 ring-primary' : 'hover:shadow-card-hover'"
          @click="selectFolder(folder.id)"
        >
          <div class="flex items-start gap-2">
            <div class="w-16 h-12 rounded bg-surface-muted overflow-hidden shrink-0">
              <img v-if="folder.cover_url" :src="folder.cover_url" :alt="folder.title" class="w-full h-full object-cover" />
              <Folder v-else :size="20" class="w-full h-full p-2 text-ink-muted" />
            </div>
            <div class="flex-1 min-w-0">
              <!-- 编辑态 -->
              <div v-if="editingId === folder.id" class="flex flex-col gap-1" @click.stop>
                <div class="flex items-center gap-1">
                  <input
                    v-model="editingTitle"
                    type="text"
                    maxlength="30"
                    class="flex-1 min-w-0 h-7 px-2 text-sm bg-surface-subtle rounded focus:outline-none focus:ring-2 focus:ring-primary/30"
                    @keyup.enter="saveEdit"
                    @keyup.esc="cancelEdit"
                  />
                  <button class="text-primary hover:text-primary-dark" @click="saveEdit"><Check :size="14" /></button>
                  <button class="text-ink-muted hover:text-ink" @click="cancelEdit"><X :size="14" /></button>
                </div>
                <div class="flex items-center gap-1 mt-1">
                  <span class="text-xs text-ink-muted">封面：</span>
                  <button v-if="!editCoverPreview" class="text-xs text-primary hover:underline" @click="editCoverInput?.click()">上传</button>
                  <div v-else class="relative inline-block">
                    <img :src="editCoverPreview" alt="封面" class="w-10 h-8 object-cover rounded" />
                    <button class="absolute -top-1 -right-1 w-3.5 h-3.5 bg-black/60 rounded-full text-white flex items-center justify-center text-[9px]" @click="clearEditCover">×</button>
                  </div>
                  <input ref="editCoverInput" type="file" accept="image/*" class="hidden" @change="handleEditCoverChange" />
                </div>
              </div>
              <!-- 展示态 -->
              <template v-else>
                <div class="text-sm font-medium text-ink truncate">{{ folder.title }}</div>
                <div class="text-xs text-ink-muted mt-0.5">{{ folder.video_count }} 个视频</div>
              </template>
            </div>
          </div>
          <!-- 操作按钮 -->
          <div v-if="editingId !== folder.id" class="mt-2 flex items-center gap-2 opacity-0 group-hover:opacity-100 transition" @click.stop>
            <button class="text-xs text-ink-muted hover:text-primary flex items-center gap-0.5" @click="startEdit(folder)">
              <Pencil :size="12" /> 重命名
            </button>
            <button
              v-if="!folder.is_default"
              class="text-xs text-ink-muted hover:text-primary flex items-center gap-0.5"
              @click="handleDelete(folder)"
            >
              <Trash2 :size="12" /> 删除
            </button>
            <span v-else class="text-xs text-ink-muted">默认夹</span>
          </div>
        </div>
        <EmptyState v-if="!loadingFolders && !folders.length" text="还没有收藏夹" />
      </div>

      <!-- 视频列表 -->
      <div class="flex-1 min-w-0">
        <div class="bg-white rounded-card shadow-card p-4">
          <div v-if="selectedFolder" class="text-sm text-ink-secondary mb-3">
            「{{ selectedFolder.title }}」共 {{ total }} 个视频
          </div>
          <div v-if="loadingVideos" class="py-12 text-center text-sm text-ink-muted">加载中...</div>
          <template v-else-if="videos.length">
            <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
              <div v-for="video in videos" :key="video.id" class="relative group">
                <VideoCard :video="video" />
                <!-- 悬停操作 -->
                <div class="absolute top-1 right-1 flex gap-1 opacity-0 group-hover:opacity-100 transition">
                  <button
                    v-if="movableFolders.length"
                    class="w-7 h-7 flex items-center justify-center bg-black/60 text-white rounded hover:bg-black/80"
                    title="移动到其他收藏夹"
                    :disabled="removing === video.id"
                    @click.stop="openMove(video)"
                  >
                    <FolderInput :size="14" />
                  </button>
                  <button
                    v-if="!selectedFolder?.is_default"
                    class="w-7 h-7 flex items-center justify-center bg-black/60 text-white rounded hover:bg-red-500"
                    title="移除收藏"
                    :disabled="removing === video.id"
                    @click.stop="handleRemove(video)"
                  >
                    <X :size="14" />
                  </button>
                </div>
              </div>
            </div>
            <Pagination :current="page" :total="total" :page-size="pageSize" @change="onPageChange" />
          </template>
          <EmptyState v-else :text="selectedFolder ? '该收藏夹暂无视频' : '请选择左侧收藏夹'" />
        </div>
      </div>
    </div>

    <!-- 移动收藏弹窗 -->
    <Teleport to="body">
      <div
        v-if="moveTargetVideo"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
        @click.self="cancelMove"
      >
        <div class="bg-white rounded-xl shadow-card p-6 w-80 max-w-[90vw]">
          <h3 class="text-base font-medium text-ink mb-1">移动收藏</h3>
          <p class="text-xs text-ink-muted mb-4 truncate">「{{ moveTargetVideo.title }}」</p>
          <div class="space-y-1 max-h-60 overflow-y-auto">
            <label
              v-for="f in movableFolders"
              :key="f.id"
              class="flex items-center gap-2 px-3 py-2 rounded hover:bg-surface-subtle cursor-pointer"
            >
              <input
                v-model="moveDestFolderId"
                type="radio"
                :value="f.id"
                class="text-primary"
              />
              <span class="text-sm">{{ f.title }}</span>
              <span class="text-xs text-ink-muted ml-auto">{{ f.video_count }} 个</span>
            </label>
            <div v-if="!movableFolders.length" class="text-sm text-ink-muted py-4 text-center">
              没有其他收藏夹可移动
            </div>
          </div>
          <div class="flex gap-2 mt-4">
            <button
              class="flex-1 h-9 rounded bg-surface-muted text-ink-secondary text-sm hover:bg-surface-muted/70"
              @click="cancelMove"
            >取消</button>
            <button
              class="flex-1 h-9 rounded bg-primary text-white text-sm hover:bg-primary-dark disabled:opacity-50"
              :disabled="moving || moveDestFolderId === null || !movableFolders.length"
              @click="confirmMove"
            >{{ moving ? '移动中...' : '确认移动' }}</button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>
