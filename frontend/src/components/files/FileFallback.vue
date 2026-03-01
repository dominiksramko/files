<script setup lang="ts">
import { computed } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { fileCategory, formatSize } from '@/utils/formatters'

const props = defineProps<{ entry: FileEntry }>()

const categoryStyle: Record<string, { bg: string; icon: string }> = {
  office: { bg: 'bg-blue-950/60', icon: 'text-blue-400' },
  text:   { bg: 'bg-zinc-800/60', icon: 'text-zinc-300' },
  other:  { bg: 'bg-zinc-800/60', icon: 'text-zinc-500' },
}

const catStyle = computed(() => {
  const cat = fileCategory(props.entry.mime, props.entry.name)
  return categoryStyle[cat] ?? { bg: 'bg-zinc-800/60', icon: 'text-zinc-500' }
})
</script>

<template>
  <div class="flex flex-col items-center gap-6 p-8 w-full" @click.stop>
    <div :class="['size-24 rounded-2xl flex items-center justify-center', catStyle.bg]">
      <svg
        :class="['size-12', catStyle.icon]"
        fill="none" viewBox="0 0 24 24" stroke="currentColor"
      >
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
          d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
      </svg>
    </div>
    <div class="text-center space-y-1">
      <p class="text-white font-semibold text-lg leading-tight break-all">{{ entry.name }}</p>
      <p class="text-zinc-400 text-sm">{{ formatSize(entry.size) }}</p>
      <p class="text-zinc-600 text-xs font-mono">{{ entry.mime || 'Unknown type' }}</p>
    </div>
    <a
      :href="filesApi.downloadUrl(entry.path, entry.mod_time)"
      target="_blank"
      class="flex items-center gap-2 bg-blue-600 hover:bg-blue-700 text-white font-medium text-sm px-6 py-2.5 rounded-xl transition-colors"
      @click.stop
    >
      <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
          d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
      </svg>
      Download
    </a>
  </div>
</template>
