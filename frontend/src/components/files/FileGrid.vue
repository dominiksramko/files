<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import type { FileEntry } from '@/types'
import { fileCategory, formatSize } from '@/utils/formatters'
import { files as filesApi } from '@/utils/api'
import AppSpinner from '@/components/ui/AppSpinner.vue'

const props = defineProps<{
  entries: FileEntry[]
  loading: boolean
  selected: string[]
  thumbHashes: Map<string, string>
}>()
const emit = defineEmits<{
  navigate: [path: string]
  preview: [entry: FileEntry]
  archivePreview: [entry: FileEntry]
  toggleSelect: [path: string]
  renameStart: [entry: FileEntry]
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

function download(entry: FileEntry, e: Event) {
  e.stopPropagation()
  window.open(filesApi.downloadUrl(entry.path, entry.mod_time), '_blank')
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
        :class="[
          'group border rounded-xl overflow-hidden cursor-pointer transition-all select-none',
          selected.includes(item.entry.path)
            ? 'bg-blue-950/60 border-blue-500 ring-2 ring-blue-500/40'
            : 'bg-zinc-900 border-zinc-800 hover:border-zinc-600 hover:bg-zinc-800/80',
        ]"
        @click="handleClick(item.entry)"
        @dragstart.prevent
      >
        <!-- Thumbnail area (square) -->
        <div
          :style="{ height: item.thumbH + 'px' }"
          class="overflow-hidden relative"
        >
          <!-- Image: show thumbnail when ready, placeholder icon while generating -->
          <img
            v-if="!item.entry.is_dir && item.entry.mime.startsWith('image/') && thumbHashes.has(item.entry.path)"
            :src="filesApi.thumbUrl(item.entry.path, thumbHashes.get(item.entry.path)!)"
            :alt="item.entry.name"
            draggable="false"
            class="w-full h-full object-cover"
          />
          <div
            v-else-if="!item.entry.is_dir && item.entry.mime.startsWith('image/')"
            class="w-full h-full flex items-center justify-center bg-green-950/60"
          >
            <svg class="size-12 text-green-900" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
            </svg>
          </div>

          <!-- Video: thumbnail when ready, icon placeholder while generating -->
          <img
            v-else-if="!item.entry.is_dir && item.entry.mime.startsWith('video/') && thumbHashes.has(item.entry.path)"
            :src="filesApi.thumbUrl(item.entry.path, thumbHashes.get(item.entry.path)!)"
            :alt="item.entry.name"
            draggable="false"
            class="w-full h-full object-cover"
          />
          <div
            v-else-if="!item.entry.is_dir && item.entry.mime.startsWith('video/')"
            class="w-full h-full flex items-center justify-center bg-purple-950/60"
          >
            <svg class="size-12 text-purple-900" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                d="M15 10l4.553-2.276A1 1 0 0121 8.723v6.554a1 1 0 01-1.447.894L15 14M3 8a2 2 0 012-2h8a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2V8z" />
            </svg>
          </div>

          <!-- Audio -->
          <div
            v-else-if="!item.entry.is_dir && item.entry.mime.startsWith('audio/')"
            class="w-full h-full flex items-center justify-center bg-pink-950/60"
          >
            <svg class="size-12 text-pink-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3" />
            </svg>
          </div>

          <!-- PDF -->
          <div
            v-else-if="!item.entry.is_dir && item.entry.mime === 'application/pdf'"
            class="w-full h-full flex items-center justify-center bg-red-950/60"
          >
            <svg class="size-12 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z" />
            </svg>
          </div>

          <!-- Archive -->
          <div
            v-else-if="!item.entry.is_dir && fileCategory(item.entry.mime, item.entry.name) === 'archive'"
            class="w-full h-full flex items-center justify-center bg-orange-950/60"
          >
            <svg class="size-12 text-orange-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
            </svg>
          </div>

          <!-- Folder -->
          <div
            v-else-if="item.entry.is_dir"
            class="w-full h-full flex items-center justify-center bg-yellow-950/60"
          >
            <svg class="size-14 text-yellow-400" fill="currentColor" viewBox="0 0 24 24">
              <path d="M2 6a2 2 0 012-2h5l2 2h9a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2V6z" />
            </svg>
          </div>

          <!-- Generic fallback -->
          <div
            v-else
            :class="['w-full h-full flex items-center justify-center', catBg[fileCategory(item.entry.mime, item.entry.name)] ?? catBg['other']]"
          >
            <svg
              :class="['size-12', catIcon[fileCategory(item.entry.mime, item.entry.name)] ?? catIcon['other']]"
              fill="none" viewBox="0 0 24 24" stroke="currentColor"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
            </svg>
          </div>

          <!-- Rename button (top-right of thumbnail, hover only) -->
          <button
            class="absolute top-1 right-1 z-10 size-7 flex items-center justify-center rounded-lg opacity-0 group-hover:opacity-100 transition-opacity bg-black/60 hover:bg-black/80 text-white backdrop-blur-sm"
            title="Rename"
            @click.stop="emit('renameStart', item.entry)"
          >
            <svg class="size-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
            </svg>
          </button>

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

          <!-- Download button (hover, files only) -->
          <button
            v-if="!item.entry.is_dir"
            class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity bg-black/60 hover:bg-black/80 text-white rounded-lg p-1.5 backdrop-blur-sm"
            title="Download"
            @click="download(item.entry, $event)"
          >
            <svg class="size-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
            </svg>
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
