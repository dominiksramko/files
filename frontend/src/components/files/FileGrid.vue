<script setup lang="ts">
import type { FileEntry } from '@/types'
import { fileCategory, formatSize, isPreviewable } from '@/utils/formatters'
import { files as filesApi } from '@/utils/api'
import AppSpinner from '@/components/ui/AppSpinner.vue'

defineProps<{
  entries: FileEntry[]
  loading: boolean
}>()

const emit = defineEmits<{
  navigate: [path: string]
  preview: [entry: FileEntry]
  archivePreview: [entry: FileEntry]
}>()

function handleClick(entry: FileEntry) {
  if (entry.is_dir) {
    emit('navigate', entry.path)
  } else if (fileCategory(entry.mime, entry.name) === 'archive') {
    emit('archivePreview', entry)
  } else if (isPreviewable(entry.mime)) {
    emit('preview', entry)
  } else {
    window.open(filesApi.downloadUrl(entry.path), '_blank')
  }
}

function download(entry: FileEntry, e: Event) {
  e.stopPropagation()
  window.open(filesApi.downloadUrl(entry.path), '_blank')
}

const categoryBg: Record<string, string> = {
  folder: 'bg-yellow-950/60',
  image: 'bg-green-950/60',
  video: 'bg-purple-950/60',
  audio: 'bg-pink-950/60',
  pdf: 'bg-red-950/60',
  office: 'bg-blue-950/60',
  archive: 'bg-orange-950/60',
  text: 'bg-zinc-800/60',
  other: 'bg-zinc-800/60',
}

const categoryIcon: Record<string, string> = {
  folder: 'text-yellow-400',
  image: 'text-green-400',
  video: 'text-purple-400',
  audio: 'text-pink-400',
  pdf: 'text-red-400',
  office: 'text-blue-400',
  archive: 'text-orange-400',
  text: 'text-zinc-300',
  other: 'text-zinc-500',
}
</script>

<template>
  <!-- Loading -->
  <div v-if="loading" class="flex items-center justify-center h-full">
    <AppSpinner size="lg" class="text-blue-500" />
  </div>

  <!-- Empty -->
  <div
    v-else-if="entries.length === 0"
    class="flex flex-col items-center justify-center h-full gap-2 text-zinc-500"
  >
    <svg class="size-12" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="1"
        d="M2 6a2 2 0 012-2h5l2 2h9a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2V6z"
      />
    </svg>
    <span class="text-sm">This folder is empty</span>
  </div>

  <!-- Grid -->
  <div
    v-else
    class="h-full overflow-y-auto p-4"
  >
    <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3">
      <div
        v-for="entry in entries"
        :key="entry.path"
        class="group relative bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden cursor-pointer hover:border-zinc-600 hover:bg-zinc-800/80 transition-all select-none"
        @click="handleClick(entry)"
      >
        <!-- Thumbnail area -->
        <div class="aspect-square w-full overflow-hidden relative">

          <!-- Image thumbnail -->
          <img
            v-if="!entry.is_dir && entry.mime.startsWith('image/')"
            :src="filesApi.streamUrl(entry.path)"
            :alt="entry.name"
            class="w-full h-full object-cover"
            loading="lazy"
          />

          <!-- Video thumbnail (icon-based, no auto-load) -->
          <div
            v-else-if="!entry.is_dir && entry.mime.startsWith('video/')"
            :class="['w-full h-full flex items-center justify-center', categoryBg['video']]"
          >
            <svg class="size-12 text-purple-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                d="M15 10l4.553-2.276A1 1 0 0121 8.723v6.554a1 1 0 01-1.447.894L15 14M3 8a2 2 0 012-2h8a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2V8z" />
            </svg>
          </div>

          <!-- Audio -->
          <div
            v-else-if="!entry.is_dir && entry.mime.startsWith('audio/')"
            :class="['w-full h-full flex items-center justify-center', categoryBg['audio']]"
          >
            <svg class="size-12 text-pink-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3" />
            </svg>
          </div>

          <!-- PDF -->
          <div
            v-else-if="!entry.is_dir && entry.mime === 'application/pdf'"
            :class="['w-full h-full flex items-center justify-center', categoryBg['pdf']]"
          >
            <svg class="size-12 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z" />
            </svg>
          </div>

          <!-- Archive -->
          <div
            v-else-if="!entry.is_dir && fileCategory(entry.mime, entry.name) === 'archive'"
            :class="['w-full h-full flex items-center justify-center', categoryBg['archive']]"
          >
            <svg class="size-12 text-orange-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
            </svg>
          </div>

          <!-- Folder -->
          <div
            v-else-if="entry.is_dir"
            :class="['w-full h-full flex items-center justify-center', categoryBg['folder']]"
          >
            <svg class="size-14 text-yellow-400" fill="currentColor" viewBox="0 0 24 24">
              <path d="M2 6a2 2 0 012-2h5l2 2h9a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2V6z" />
            </svg>
          </div>

          <!-- Office -->
          <div
            v-else-if="fileCategory(entry.mime, entry.name) === 'office'"
            :class="['w-full h-full flex items-center justify-center', categoryBg['office']]"
          >
            <svg class="size-12 text-blue-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
            </svg>
          </div>

          <!-- Generic file -->
          <div
            v-else
            :class="['w-full h-full flex items-center justify-center', categoryBg[fileCategory(entry.mime, entry.name)] ?? categoryBg['other']]"
          >
            <svg
              :class="['size-12', categoryIcon[fileCategory(entry.mime, entry.name)] ?? categoryIcon['other']]"
              fill="none" viewBox="0 0 24 24" stroke="currentColor"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
            </svg>
          </div>

          <!-- Download button (hover, files only) -->
          <button
            v-if="!entry.is_dir"
            class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity bg-black/60 hover:bg-black/80 text-white rounded-lg p-1.5 backdrop-blur-sm"
            title="Download"
            @click="download(entry, $event)"
          >
            <svg class="size-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
            </svg>
          </button>
        </div>

        <!-- Name + meta -->
        <div class="px-2.5 py-2">
          <p class="text-xs font-medium text-zinc-100 truncate leading-tight">{{ entry.name }}</p>
          <p class="text-xs text-zinc-500 mt-0.5 tabular-nums">
            {{ entry.is_dir ? 'Folder' : formatSize(entry.size) }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
