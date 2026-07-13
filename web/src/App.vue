<script setup lang="ts">
import TheNavbar from '@/components/layout/TheNavbar.vue'
</script>

<template>
  <div class="min-h-screen flex flex-col bg-surface-subtle">
    <TheNavbar />
    <main class="flex-1">
      <router-view v-slot="{ Component }">
        <transition name="fade-slide" mode="out-in">
          <component :is="Component" />
        </transition>
      </router-view>
    </main>
  </div>
</template>
