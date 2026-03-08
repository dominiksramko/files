<script setup lang="ts">
import AppIcon from '@/components/ui/AppIcon.vue'

defineProps<{
  selectedCount: number
  clipboard: { mode: 'copy' | 'move'; paths: string[] } | null
  downloading: boolean
  canExtract: boolean
}>()

const emit = defineEmits<{
  copy: []
  move: []
  download: []
  compress: []
  extract: []
  rename: []
  delete: []
  details: []
  paste: []
  clearClipboard: []
  deselectAll: []
}>()
</script>

<template>
  <template v-if="selectedCount > 0">
    <!-- Deselect + count -->
    <button
      class="flex items-center gap-1.5 text-sm text-zinc-200 hover:text-white shrink-0 pl-0.5 pr-2 py-1 rounded hover:bg-zinc-800 transition-colors"
      title="Clear selection"
      @click="emit('deselectAll')"
    >
      <AppIcon name="x" class="size-4 text-zinc-400" />
      {{ selectedCount }} selected
    </button>

    <div class="w-px h-4 bg-zinc-700 mx-0.5 shrink-0" />

    <button class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 transition-colors shrink-0" title="Copy" @click="emit('copy')">
      <AppIcon name="copy" class="size-4" />
    </button>
    <button class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 transition-colors shrink-0" title="Move" @click="emit('move')">
      <AppIcon name="arrowRight" class="size-4" />
    </button>
    <button
      :disabled="downloading"
      class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 disabled:opacity-40 transition-colors shrink-0"
      :title="downloading ? 'Zipping…' : 'Download'"
      @click="emit('download')"
    >
      <AppIcon name="download" class="size-4" />
    </button>
    <button class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 transition-colors shrink-0" title="Compress" @click="emit('compress')">
      <AppIcon name="archive" class="size-4" />
    </button>
    <button v-if="canExtract" class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 transition-colors shrink-0" title="Extract here" @click="emit('extract')">
      <AppIcon name="archiveOut" class="size-4" />
    </button>
    <button v-if="selectedCount === 1" class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 transition-colors shrink-0" title="Rename" @click="emit('rename')">
      <AppIcon name="pencil" class="size-4" />
    </button>
    <button class="p-1.5 rounded text-zinc-400 hover:text-red-400 hover:bg-zinc-700 transition-colors shrink-0" title="Delete" @click="emit('delete')">
      <AppIcon name="trash" class="size-4" />
    </button>
    <button class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 transition-colors shrink-0" title="Details" @click="emit('details')">
      <AppIcon name="info" class="size-4" />
    </button>
  </template>

  <!-- Separator between selection and clipboard -->
  <div v-if="selectedCount > 0 && clipboard" class="w-px h-4 bg-zinc-700 mx-0.5 shrink-0" />

  <template v-if="clipboard">
    <span class="text-xs text-zinc-400 italic shrink-0 truncate">
      {{ clipboard.mode === 'copy' ? 'Copying' : 'Moving' }}
      {{ clipboard.paths.length }} item{{ clipboard.paths.length !== 1 ? 's' : '' }}
    </span>
    <button
      class="p-1.5 rounded text-blue-400 hover:text-blue-300 hover:bg-zinc-700 transition-colors shrink-0"
      title="Paste here"
      @click="emit('paste')"
    >
      <AppIcon name="clipboard" class="size-4" />
    </button>
    <button
      class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors shrink-0"
      title="Clear clipboard"
      @click="emit('clearClipboard')"
    >
      <AppIcon name="x" class="size-4" />
    </button>
  </template>
</template>
