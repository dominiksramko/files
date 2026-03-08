<script setup lang="ts">
import { computed, ref } from 'vue'
import type { FileEntry } from '@/types'
import { fileCategory, formatSize, formatDate, setMoveDragImage } from '@/utils/formatters'
import FileIcon from './FileIcon.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import AppCheckbox from '@/components/ui/AppCheckbox.vue'
import AppIcon from '@/components/ui/AppIcon.vue'

const props = defineProps<{
  entries: FileEntry[]
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
  contextMenu: [entry: FileEntry, x: number, y: number]
  dropToFolder: [destPath: string, paths: string[]]
}>()

const allSelected = computed(
  () => props.entries.length > 0 && props.entries.every((e) => props.selected.includes(e.path)),
)

const someSelected = computed(
  () => props.selected.length > 0 && !allSelected.value,
)

function onHeaderCheckbox() {
  if (allSelected.value) emit('deselectAll')
  else emit('selectAll')
}

function handleClick(entry: FileEntry) {
  if (props.selected.length > 0) {
    emit('toggleSelect', entry.path)
    return
  }
  if (entry.is_dir) emit('navigate', entry.path)
  else if (fileCategory(entry.mime, entry.name) === 'archive') emit('archivePreview', entry)
  else emit('preview', entry)
}

// ── Drag & drop ──────────────────────────────────────────────────────────────
const dragOverPath = ref<string | null>(null)

function onRowDragStart(e: DragEvent, entry: FileEntry) {
  const paths = props.selected.includes(entry.path) ? [...props.selected] : [entry.path]
  e.dataTransfer!.setData('application/x-files-internal', JSON.stringify(paths))
  e.dataTransfer!.effectAllowed = 'move'
  setMoveDragImage(e, paths.length)
}

function onRowDragEnd() {
  dragOverPath.value = null
}

function onRowDragOver(e: DragEvent, entry: FileEntry) {
  if (!entry.is_dir) return
  if (!e.dataTransfer?.types.includes('application/x-files-internal')) return
  e.preventDefault()
  e.dataTransfer.dropEffect = 'move'
  dragOverPath.value = entry.path
}

function onRowDragLeave(e: DragEvent, entry: FileEntry) {
  if (!entry.is_dir) return
  const related = e.relatedTarget as Node | null
  const row = e.currentTarget as HTMLElement
  if (!related || !row.contains(related)) {
    if (dragOverPath.value === entry.path) dragOverPath.value = null
  }
}

function onRowDrop(e: DragEvent, entry: FileEntry) {
  if (!entry.is_dir) return
  e.preventDefault()
  dragOverPath.value = null
  const raw = e.dataTransfer?.getData('application/x-files-internal')
  if (!raw) return
  const paths: string[] = JSON.parse(raw)
  emit('dropToFolder', entry.path, paths)
}
</script>

<template>
  <div class="h-full flex flex-col">
    <div v-if="loading" class="flex-1 flex items-center justify-center">
      <AppSpinner size="lg" class="text-blue-500" />
    </div>

    <div
      v-else-if="entries.length === 0"
      class="flex-1 flex flex-col items-center justify-center gap-2 text-zinc-500"
    >
      <AppIcon name="folderOutline" class="size-12" :stroke-width="1" />
      <span class="text-sm">This folder is empty</span>
    </div>

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
                <AppCheckbox
                  :model-value="allSelected"
                  :indeterminate="someSelected && !allSelected"
                  @update:model-value="onHeaderCheckbox"
                />
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
              class="text-right px-4 py-2.5 font-medium text-zinc-400 cursor-pointer hover:text-white select-none w-24 hidden sm:table-cell"
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
              <span v-if="sortKey === 'mod_time'" class="ml-1 text-xs">{{ sortDesc ? '↓' : '↑' }}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="entry in entries"
            :key="entry.path"
            draggable="true"
            :class="[
              'border-b border-zinc-800/50 cursor-pointer group transition-colors',
              entry.is_dir && dragOverPath === entry.path
                ? 'bg-blue-900/30 outline outline-1 outline-blue-500/40'
                : selected.includes(entry.path)
                  ? 'bg-blue-950/40 hover:bg-blue-950/60'
                  : 'hover:bg-zinc-800/50',
            ]"
            @click="handleClick(entry)"
            @contextmenu.prevent="emit('contextMenu', entry, $event.clientX, $event.clientY)"
            @dragstart="onRowDragStart($event, entry)"
            @dragend="onRowDragEnd"
            @dragover="onRowDragOver($event, entry)"
            @dragleave="onRowDragLeave($event, entry)"
            @drop="onRowDrop($event, entry)"
          >
            <!-- Custom checkbox -->
            <td class="w-10 cursor-pointer" @click.stop="emit('toggleSelect', entry.path)">
              <div class="flex items-center justify-center h-10">
                <AppCheckbox
                  :model-value="selected.includes(entry.path)"
                  @update:model-value="emit('toggleSelect', entry.path)"
                />
              </div>
            </td>

            <!-- Name cell -->
            <td class="px-2 py-2">
              <div class="flex items-center gap-2.5 min-w-0">
                <FileIcon :mime="entry.mime" :name="entry.name" :is-dir="entry.is_dir" />
                <span class="truncate text-zinc-100">{{ entry.name }}</span>
                <span
                  v-if="!entry.is_dir && fileCategory(entry.mime, entry.name) === 'archive'"
                  class="shrink-0 text-xs bg-orange-950 text-orange-300 border border-orange-900 rounded px-1.5 py-0.5"
                >archive</span>
              </div>
            </td>

            <td class="px-4 py-2.5 text-right text-zinc-400 tabular-nums whitespace-nowrap hidden sm:table-cell">
              {{ entry.is_dir ? '—' : formatSize(entry.size) }}
            </td>
            <td class="px-4 py-2.5 text-right text-zinc-500 text-xs whitespace-nowrap hidden sm:table-cell">
              {{ formatDate(entry.mod_time) }}
            </td>

          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
