<script setup lang="ts">
import type { FileEntry } from '@/types'
import { fileCategory, formatSize, formatDate } from '@/utils/formatters'
import { files as filesApi } from '@/utils/api'
import FileIcon from './FileIcon.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import AppIcon from '@/components/ui/AppIcon.vue'

const props = defineProps<{
  entries: FileEntry[]
  loading: boolean
  query: string
}>()

const emit = defineEmits<{
  navigate: [path: string]
  preview: [entry: FileEntry]
  archivePreview: [entry: FileEntry]
}>()

function parentDir(path: string): string {
  const parts = path.replace(/\\/g, '/').split('/')
  parts.pop()
  return parts.length > 0 ? parts.join('/') : ''
}

function handleClick(entry: FileEntry) {
  if (entry.is_dir) emit('navigate', entry.path)
  else if (fileCategory(entry.mime, entry.name) === 'archive') emit('archivePreview', entry)
  else emit('preview', entry)
}

function download(entry: FileEntry, e: Event) {
  e.stopPropagation()
  window.open(filesApi.downloadUrl(entry.path, entry.mod_time), '_blank')
}

function highlight(name: string, q: string): string {
  if (!q) return esc(name)
  const idx = name.toLowerCase().indexOf(q.toLowerCase())
  if (idx === -1) return esc(name)
  return (
    esc(name.slice(0, idx)) +
    `<mark class="bg-yellow-400/30 text-yellow-200 rounded-sm">${esc(name.slice(idx, idx + q.length))}</mark>` +
    esc(name.slice(idx + q.length))
  )
}

function esc(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}
</script>

<template>
  <div class="h-full flex flex-col">
    <!-- Loading with no results yet -->
    <div v-if="loading && entries.length === 0" class="flex-1 flex items-center justify-center gap-2 text-zinc-400 text-sm">
      <AppSpinner size="sm" class="text-blue-500" />
      Searching…
    </div>

    <!-- No results -->
    <div
      v-else-if="!loading && entries.length === 0"
      class="flex-1 flex flex-col items-center justify-center gap-2 text-zinc-500"
    >
      <AppIcon name="search" class="size-10" :stroke-width="1.5" />
      <span class="text-sm">No results for <span class="text-white">"{{ query }}"</span></span>
    </div>

    <!-- Results -->
    <template v-else>
      <!-- Result count banner -->
      <div class="shrink-0 px-4 py-1.5 text-xs text-zinc-500 bg-zinc-900 border-b border-zinc-800 flex items-center gap-2">
        <AppSpinner v-if="loading" size="sm" class="text-blue-500 shrink-0" />
        {{ entries.length }}{{ entries.length === 200 ? '+' : '' }}
        result{{ entries.length !== 1 ? 's' : '' }} for
        <span class="text-zinc-300">"{{ query }}"</span>
      </div>

      <div class="flex-1 overflow-auto">
        <table class="w-full text-sm">
          <tbody>
            <tr
              v-for="entry in entries"
              :key="entry.path"
              class="border-b border-zinc-800/50 hover:bg-zinc-800/50 cursor-pointer group transition-colors"
              @click="handleClick(entry)"
            >
              <!-- Icon + name + parent path -->
              <td class="px-4 py-2.5">
                <div class="flex items-center gap-2.5 min-w-0">
                  <FileIcon :mime="entry.mime" :name="entry.name" :is-dir="entry.is_dir" />
                  <div class="min-w-0">
                    <!-- eslint-disable-next-line vue/no-v-html -->
                    <p class="truncate text-zinc-100 leading-tight" v-html="highlight(entry.name, query)" />
                    <p class="text-xs text-zinc-600 truncate leading-tight mt-0.5">
                      {{ parentDir(entry.path) || '/' }}
                    </p>
                  </div>
                </div>
              </td>
              <!-- Size -->
              <td class="px-4 py-2.5 text-right text-zinc-400 tabular-nums w-24 shrink-0">
                {{ entry.is_dir ? '—' : formatSize(entry.size) }}
              </td>
              <!-- Modified -->
              <td class="px-4 py-2.5 text-right text-zinc-500 text-xs w-44 hidden sm:table-cell">
                {{ formatDate(entry.mod_time) }}
              </td>
              <!-- Actions -->
              <td class="px-2 py-2.5 w-20">
                <div class="flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                  <button
                    class="p-1 rounded hover:bg-zinc-700 text-zinc-400 hover:text-white"
                    title="Open containing folder"
                    @click.stop="emit('navigate', parentDir(entry.path))"
                  >
                    <AppIcon name="folder" class="size-4" />
                  </button>
                  <button
                    v-if="!entry.is_dir"
                    class="p-1 rounded hover:bg-zinc-700 text-zinc-400 hover:text-white"
                    title="Download"
                    @click.stop="download(entry, $event)"
                  >
                    <AppIcon name="download" class="size-4" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>
