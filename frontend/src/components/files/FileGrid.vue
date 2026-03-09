<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import type { FileEntry } from '@/types'
import { fileCategory, formatSize, setMoveDragImage } from '@/utils/formatters'
import type { IconName } from '@/utils/icons'
import { files as filesApi } from '@/utils/api'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import AppIcon from '@/components/ui/AppIcon.vue'

const props = defineProps<{
  entries: FileEntry[]
  loading: boolean
  selected: string[]
}>()
const emit = defineEmits<{
  navigate: [path: string]
  preview: [entry: FileEntry]
  archivePreview: [entry: FileEntry]
  toggleSelect: [path: string]
  contextMenu: [entry: FileEntry, x: number, y: number]
  dropToFolder: [destPath: string, paths: string[]]
}>()

// ── Virtual scroll constants ────────────────────────────────────────────────
const PADDING = 16   // matches p-4
const GAP = 12       // matches gap-3
const META_H = 52    // fixed height of the name+size area below thumbnail
const OVERSCAN = 2   // extra rows rendered above and below the viewport

// ── Measurements ────────────────────────────────────────────────────────────
const containerRef = ref<HTMLElement | null>(null)
const scrollTop = ref(0)
const vpW = ref(window.innerWidth)
const vpH = ref(window.innerHeight)

// Match Tailwind breakpoints used in the old grid
function calcCols(w: number): number {
  if (w >= 1280) return 6
  if (w >= 1024) return 5
  if (w >= 768) return 4
  if (w >= 640) return 3
  return 2
}

// Inner width after padding
const innerW = computed(() => vpW.value - PADDING * 2)
const numCols = computed(() => calcCols(innerW.value))
const itemW = computed(() => (innerW.value - GAP * (numCols.value - 1)) / numCols.value)
// Square thumbnail + fixed meta area
const itemH = computed(() => itemW.value + META_H)
const rowH = computed(() => itemH.value + GAP)
const numRows = computed(() => Math.ceil(props.entries.length / numCols.value))
// Total scrollable height (padding top + all rows + padding bottom)
const totalH = computed(() => PADDING + numRows.value * rowH.value - GAP + PADDING)

// ── Visible slice ────────────────────────────────────────────────────────────
const visibleItems = computed(() => {
  const firstRow = Math.max(0, Math.floor((scrollTop.value - PADDING) / rowH.value) - OVERSCAN)
  const visRowCount = Math.ceil(vpH.value / rowH.value) + OVERSCAN * 2 + 1
  const lastRow = Math.min(numRows.value, firstRow + visRowCount)

  const items: Array<{
    entry: FileEntry
    top: number
    left: number
    w: number
    h: number
    thumbH: number
  }> = []

  for (let row = firstRow; row < lastRow; row++) {
    for (let col = 0; col < numCols.value; col++) {
      const idx = row * numCols.value + col
      if (idx >= props.entries.length) break
      items.push({
        entry: props.entries[idx]!,
        top: PADDING + row * rowH.value,
        left: PADDING + col * (itemW.value + GAP),
        w: itemW.value,
        h: itemH.value,
        thumbH: itemW.value, // square
      })
    }
  }
  return items
})

// ── DOM plumbing ─────────────────────────────────────────────────────────────
function onScroll() {
  scrollTop.value = containerRef.value?.scrollTop ?? 0
}

watch(containerRef, (el, _prev, onCleanup) => {
  if (!el) return
  vpW.value = el.clientWidth
  vpH.value = el.clientHeight
  const ro = new ResizeObserver(() => {
    vpW.value = el.clientWidth
    vpH.value = el.clientHeight
  })
  ro.observe(el)
  onCleanup(() => ro.disconnect())
})

watch(() => props.entries, () => {
  scrollTop.value = 0
  if (containerRef.value) containerRef.value.scrollTop = 0
})

// ── Click handlers ───────────────────────────────────────────────────────────
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

function onCardDragStart(e: DragEvent, entry: FileEntry) {
  const paths = props.selected.includes(entry.path) ? [...props.selected] : [entry.path]
  e.dataTransfer!.setData('application/x-files-internal', JSON.stringify(paths))
  e.dataTransfer!.effectAllowed = 'move'
  setMoveDragImage(e, paths.length)
}

function onCardDragEnd() {
  dragOverPath.value = null
}

function onCardDragOver(e: DragEvent, entry: FileEntry) {
  if (!entry.is_dir) return
  if (!e.dataTransfer?.types.includes('application/x-files-internal')) return
  e.preventDefault()
  e.dataTransfer.dropEffect = 'move'
  dragOverPath.value = entry.path
}

function onCardDragLeave(e: DragEvent, entry: FileEntry) {
  if (!entry.is_dir) return
  const related = e.relatedTarget as Node | null
  const card = e.currentTarget as HTMLElement
  if (!related || !card.contains(related)) {
    if (dragOverPath.value === entry.path) dragOverPath.value = null
  }
}

function onCardDrop(e: DragEvent, entry: FileEntry) {
  if (!entry.is_dir) return
  e.preventDefault()
  dragOverPath.value = null
  const raw = e.dataTransfer?.getData('application/x-files-internal')
  if (!raw) return
  const paths: string[] = JSON.parse(raw)
  emit('dropToFolder', entry.path, paths)
}

// ── Category helpers (icon colours) ─────────────────────────────────────────
const catBg: Record<string, string> = {
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
const catIcon: Record<string, string> = {
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
const catIconName: Record<string, IconName> = {
  folder: 'folder',
  image: 'image',
  video: 'video',
  audio: 'music',
  pdf: 'pdf',
  office: 'document',
  archive: 'archive',
  text: 'document',
  other: 'document',
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
    <AppIcon name="folderOutline" class="size-12" :stroke-width="1" />
    <span class="text-sm">This folder is empty</span>
  </div>

  <!-- Virtual grid -->
  <div
    v-else
    ref="containerRef"
    class="h-full overflow-y-auto"
    @scroll.passive="onScroll"
  >
    <!-- Spacer sized to the full virtual height -->
    <div :style="{ height: totalH + 'px', position: 'relative' }">
      <!-- Only visible cards are in the DOM -->
      <div
        v-for="item in visibleItems"
        :key="item.entry.path"
        :style="{
          position: 'absolute',
          top: item.top + 'px',
          left: item.left + 'px',
          width: item.w + 'px',
          height: item.h + 'px',
          willChange: 'transform',
        }"
        draggable="true"
        :class="[
          'group border rounded-xl overflow-hidden cursor-pointer transition-all select-none',
          item.entry.is_dir && dragOverPath === item.entry.path
            ? 'bg-blue-900/40 border-blue-500 ring-2 ring-blue-500/40'
            : selected.includes(item.entry.path)
              ? 'bg-blue-950/60 border-blue-500 ring-2 ring-blue-500/40'
              : 'bg-zinc-900 border-zinc-800 hover:border-zinc-600 hover:bg-zinc-800/80',
        ]"
        @click="handleClick(item.entry)"
        @contextmenu.prevent="emit('contextMenu', item.entry, $event.clientX, $event.clientY)"
        @dragstart="onCardDragStart($event, item.entry)"
        @dragend="onCardDragEnd"
        @dragover="onCardDragOver($event, item.entry)"
        @dragleave="onCardDragLeave($event, item.entry)"
        @drop="onCardDrop($event, item.entry)"
      >
        <!-- Thumbnail area (square) -->
        <div
          :style="{ height: item.thumbH + 'px' }"
          class="overflow-hidden relative"
        >
          <!-- Thumbnail -->
          <img
            v-if="item.entry.has_thumb && item.entry.thumb_hash"
            :src="filesApi.thumbUrl(item.entry.path, item.entry.thumb_hash)"
            :alt="item.entry.name"
            draggable="false"
            class="w-full h-full object-cover"
          />
          <!-- Folder -->
          <div
            v-else-if="item.entry.is_dir"
            class="w-full h-full flex items-center justify-center bg-yellow-950/60"
          >
            <AppIcon name="folder" class="size-14 text-yellow-400" />
          </div>
          <!-- File placeholder icon -->
          <div
            v-else
            :class="['w-full h-full flex items-center justify-center', catBg[fileCategory(item.entry.mime, item.entry.name)] ?? catBg['other']]"
          >
            <AppIcon
              :name="catIconName[fileCategory(item.entry.mime, item.entry.name)] ?? 'document'"
              :class="['size-12', catIcon[fileCategory(item.entry.mime, item.entry.name)] ?? catIcon['other']]"
              :stroke-width="1.5"
            />
          </div>

          <!-- Selection checkmark (top-left): size-8 hit area, size-5 visual circle inside -->
          <button
            class="absolute top-1 left-1 z-10 size-8 flex items-center justify-center rounded-full transition-opacity"
            :class="selected.includes(item.entry.path) ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'"
            :title="selected.includes(item.entry.path) ? 'Deselect' : 'Select'"
            @click.stop="emit('toggleSelect', item.entry.path)"
          >
            <span
              class="size-5 rounded-full flex items-center justify-center backdrop-blur-sm transition-colors"
              :class="selected.includes(item.entry.path) ? 'bg-blue-500' : 'bg-black/60'"
            >
              <svg v-if="selected.includes(item.entry.path)" class="size-2.5 text-white" viewBox="0 0 10 8" fill="none">
                <path d="M1 4L3.5 6.5L9 1" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" />
              </svg>
              <svg v-else class="size-3 text-white/70" viewBox="0 0 10 10" fill="none">
                <circle cx="5" cy="5" r="4" stroke="currentColor" stroke-width="1.5" />
              </svg>
            </span>
          </button>

        </div>

        <!-- Name + meta (fixed META_H height) -->
        <div class="px-2 py-2 overflow-hidden">
          <p class="text-xs font-medium text-zinc-100 truncate leading-tight">{{ item.entry.name }}</p>
          <p class="text-xs text-zinc-500 mt-0.5 tabular-nums truncate">
            {{ item.entry.is_dir ? 'Folder' : formatSize(item.entry.size) }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
