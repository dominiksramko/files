<script setup lang="ts">
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'

const props = defineProps<{ entry: FileEntry }>()

const streamUrl = filesApi.streamUrl(props.entry.path)
</script>

<template>
  <div class="w-full max-w-md px-6" @click.stop>
    <div class="flex items-center gap-5 mb-6">
      <div class="size-20 rounded-2xl bg-pink-950/60 flex items-center justify-center shrink-0">
        <svg class="size-10 text-pink-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
            d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3" />
        </svg>
      </div>
      <p class="text-white font-medium text-lg leading-tight">{{ entry.name }}</p>
    </div>
    <audio :src="streamUrl" controls class="w-full" />
  </div>
</template>
