<script setup lang="ts">
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { formatSize } from '@/utils/formatters'

// Shows the loading / failed / too-large states of a file loaded with
// useFileContent or useEditableFile; renders the default slot once content is ready.
defineProps<{
  entry: FileEntry
  loading: boolean
  loadError: boolean
  tooLarge: boolean
}>()
</script>

<template>
  <div v-if="loading" class="flex h-32 items-center justify-center text-sm text-zinc-400">
    Loading…
  </div>
  <div v-else-if="loadError" class="flex h-32 items-center justify-center text-sm text-red-400">
    Failed to load file content.
  </div>
  <div
    v-else-if="tooLarge"
    class="flex h-32 flex-col items-center justify-center gap-3 text-sm text-zinc-400"
  >
    <p>File is too large to open ({{ formatSize(entry.size) }})</p>
    <a
      :href="filesApi.downloadUrl(entry.path, entry.mod_time)"
      target="_blank"
      class="text-blue-400 underline hover:text-blue-300"
      @click.stop
    >Download to view</a>
  </div>
  <slot v-else />
</template>
