<script setup lang="ts">
import AppHeader from './AppHeader.vue'
import { useSubtitleStyle } from '@/composables/useSubtitleStyle'

useSubtitleStyle()
</script>

<template>
  <div class="h-screen flex flex-col bg-zinc-950 text-zinc-100 overflow-hidden">
    <AppHeader />
    <main class="flex-1 overflow-hidden">
      <RouterView />
    </main>
  </div>
</template>
