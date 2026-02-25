<script setup lang="ts">
import { computed } from 'vue'
import type { FileEntry } from '@/types'
import { fileCategory, formatSize, formatDate } from '@/utils/formatters'
import { files as filesApi } from '@/utils/api'
import FileIcon from './FileIcon.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'

const props = defineProps<{
  entries: FileEntry[] // pre-sorted from parent
  loading: boolean
  currentPath: string
  sortKey: string
  sortDesc: boolean
  selected: string[]
}>()

const emit = defineEmits<{
  navigate: [path: string]
  preview: [entry: FileEntry]
  archivePreview: [entry: FileEntry]
  toggleSort: [key: string]
  toggleSelect: [path: string]
  selectAll: []
  deselectAll: []
}>()

const allSelected = computed(
  () => props.entries.length > 0 && props.entries.every((e) => props.selected.includes(e.path)),
)

const someSelected = computed(
  () => props.selected.length > 0 && !allSelected.value,
)

function onHeaderCheckbox() {
  if (allSelected.value) {
    emit('deselectAll')
  } else {
    emit('selectAll')
  }
}

function handleClick(entry: FileEntry) {
  // If any selection is active, clicks toggle selection instead of navigating
  if (props.selected.length > 0) {
    emit('toggleSelect', entry.path)
    return
  }
  if (entry.is_dir) {
    emit('navigate', entry.path)
  } else if (fileCategory(entry.mime, entry.name) === 'archive') {
    emit('archivePreview', entry)
  } else {
    emit('preview', entry)
  }
}

function download(entry: FileEntry, e: Event) {
  e.stopPropagation()
  window.open(filesApi.downloadUrl(entry.path), '_blank')
}
</script>

<template>
  <div class="h-full flex flex-col">
    <!-- Loading overlay -->
    <div v-if="loading" class="flex-1 flex items-center justify-center">
      <AppSpinner size="lg" class="text-blue-500" />
    </div>

    <!-- Empty state -->
    <div
      v-else-if="entries.length === 0"
      class="flex-1 flex flex-col items-center justify-center gap-2 text-zinc-500"
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

    <!-- Table -->
    <div v-else class="flex-1 overflow-auto">
      <table class="w-full text-sm">
        <thead class="sticky top-0 bg-zinc-900 border-b border-zinc-800 z-10">
          <tr>
            <!-- Select-all custom checkbox -->
            <th class="w-10">
              <button
                type="button"
                class="flex items-center justify-center size-10 transition-colors hover:bg-zinc-800 rounded"
                @click.stop="onHeaderCheckbox"
              >
                <span
                  class="size-[18px] rounded border-2 flex items-center justify-center transition-colors"
                  :class="allSelected
                    ? 'bg-blue-500 border-blue-500'
                    : someSelected
                      ? 'bg-zinc-700 border-zinc-500'
                      : 'border-zinc-600'"
                >
                  <!-- All selected: checkmark -->
                  <svg v-if="allSelected" class="size-2.5 text-white" viewBox="0 0 10 8" fill="none">
                    <path d="M1 4L3.5 6.5L9 1" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" />
                  </svg>
                  <!-- Some selected: dash -->
                  <span v-else-if="someSelected" class="w-2.5 h-[2px] bg-zinc-300 rounded-full" />
                </span>
              </button>
            </th>
            <th
              class="text-left px-2 py-2.5 font-medium text-zinc-400 cursor-pointer hover:text-white select-none"
              @click="emit('toggleSort', 'name')"
            >
              Name
              <span v-if="sortKey === 'name'" class="ml-1 text-xs">{{ sortDesc ? '↓' : '↑' }}</span>
            </th>
            <th
              class="text-right px-4 py-2.5 font-medium text-zinc-400 cursor-pointer hover:text-white select-none w-24"
              @click="emit('toggleSort', 'size')"
            >
              Size
              <span v-if="sortKey === 'size'" class="ml-1 text-xs">{{ sortDesc ? '↓' : '↑' }}</span>
            </th>
            <th
              class="text-right px-4 py-2.5 font-medium text-zinc-400 cursor-pointer hover:text-white select-none w-44 hidden sm:table-cell"
              @click="emit('toggleSort', 'mod_time')"
            >
              Modified
              <span v-if="sortKey === 'mod_time'" class="ml-1 text-xs">{{
                sortDesc ? '↓' : '↑'
              }}</span>
            </th>
            <th class="w-10 px-2" />
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="entry in entries"
            :key="entry.path"
            :class="[
              'border-b border-zinc-800/50 cursor-pointer group transition-colors',
              selected.includes(entry.path)
                ? 'bg-blue-950/40 hover:bg-blue-950/60'
                : 'hover:bg-zinc-800/50',
            ]"
            @click="handleClick(entry)"
          >
            <!-- Row custom checkbox: entire td is the hit target -->
            <td class="w-10 cursor-pointer" @click.stop="emit('toggleSelect', entry.path)">
              <div class="flex items-center justify-center h-10">
                <span
                  class="size-[18px] rounded border-2 flex items-center justify-center transition-colors"
                  :class="selected.includes(entry.path)
                    ? 'bg-blue-500 border-blue-500'
                    : 'border-zinc-600'"
                >
                  <svg v-if="selected.includes(entry.path)" class="size-2.5 text-white" viewBox="0 0 10 8" fill="none">
                    <path d="M1 4L3.5 6.5L9 1" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" />
                  </svg>
                </span>
              </div>
            </td>
            <td class="px-2 py-2.5">
              <div class="flex items-center gap-2.5 min-w-0">
                <FileIcon :mime="entry.mime" :name="entry.name" :is-dir="entry.is_dir" />
                <span class="truncate text-zinc-100">{{ entry.name }}</span>
                <!-- Archive badge -->
                <span
                  v-if="!entry.is_dir && fileCategory(entry.mime, entry.name) === 'archive'"
                  class="shrink-0 text-xs bg-orange-950 text-orange-300 border border-orange-900 rounded px-1.5 py-0.5"
                >
                  archive
                </span>
              </div>
            </td>
            <td class="px-4 py-2.5 text-right text-zinc-400 tabular-nums">
              {{ entry.is_dir ? '—' : formatSize(entry.size) }}
            </td>
            <td class="px-4 py-2.5 text-right text-zinc-500 text-xs hidden sm:table-cell">
              {{ formatDate(entry.mod_time) }}
            </td>
            <td class="px-2 py-2.5">
              <button
                v-if="!entry.is_dir"
                class="opacity-0 group-hover:opacity-100 transition-opacity p-1 rounded hover:bg-zinc-700 text-zinc-400 hover:text-white"
                title="Download"
                @click="download(entry, $event)"
              >
                <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"
                  />
                </svg>
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
