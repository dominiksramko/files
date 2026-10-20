<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { fileCategory, formatDate, formatDuration, formatSize, formatSizeProgress, type SizeProgress } from '@/utils/formatters'
import { parentOf } from '@/utils/paths'
import AppButton from '@/components/ui/AppButton.vue'
import AppDialog from '@/components/ui/AppDialog.vue'

const props = defineProps<{ items: FileEntry[] }>()
const emit = defineEmits<{ close: [] }>()

type MediaMeta = {
  image?: { width: number; height: number }
  video?: { duration: number; width: number; height: number; codec?: string }
}

const sizeProgress = ref<SizeProgress | null>(null)
const detailsMedia = ref<MediaMeta | null>(null)
let sizeEventSource: EventSource | null = null
// Counts media info requests, so a late answer about a previous file is dropped.
let mediaRequest = 0

const location = computed(() => {
  if (!props.items.length) return ''
  const parents = [...new Set(props.items.map((entry) => '/' + parentOf(entry.path)))]
  return parents.length === 1 ? parents[0] : 'Multiple locations'
})

const fileCount = computed(() => props.items.filter((entry) => !entry.is_dir).length)
const folderCount = computed(() => props.items.filter((entry) => entry.is_dir).length)

const sizeText = computed(() => (sizeProgress.value ? formatSizeProgress(sizeProgress.value) : null))

function stopStream() {
  sizeEventSource?.close()
  sizeEventSource = null
}

watch(() => props.items, (items) => {
  stopStream()
  sizeProgress.value = null
  detailsMedia.value = null
  const request = ++mediaRequest
  if (!items.length) return

  const first = items[0]
  const needsStream = items.length > 1 || (items.length === 1 && first?.is_dir)
  if (needsStream) {
    sizeProgress.value = { files: 0, dirs: 0, bytes: 0, done: false }
    const url = filesApi.sizeStreamUrl(items.map((entry) => entry.path))
    sizeEventSource = new EventSource(url)
    sizeEventSource.onmessage = (ev) => {
      sizeProgress.value = JSON.parse(ev.data)
      if (sizeProgress.value?.done) stopStream()
    }
    // The scan stopped before done (server error, connection lost).
    sizeEventSource.onerror = () => {
      if (sizeProgress.value && !sizeProgress.value.done) sizeProgress.value = { ...sizeProgress.value, failed: true }
      stopStream()
    }
  }

  if (first && items.length === 1 && !first.is_dir) {
    const cat = fileCategory(first.mime, first.name)
    if (cat === 'image' || cat === 'video') {
      filesApi.fileMeta(first.path).then((meta) => {
        if (request === mediaRequest) detailsMedia.value = meta
      }).catch(() => {})
    }
  }
}, { immediate: true })

onUnmounted(stopStream)
</script>

<template>
  <AppDialog :open="items.length > 0" width="w-96 max-w-[calc(100vw-2rem)]" @close="emit('close')">
    <!-- Header -->
    <h3 class="text-sm font-semibold text-zinc-100 mb-3">
      <template v-if="items.length === 1">Details</template>
      <template v-else>
        Details —
        <span v-if="folderCount > 0">{{ folderCount.toLocaleString() }} folder{{ folderCount !== 1 ? 's' : '' }}</span>
        <span v-if="folderCount > 0 && fileCount > 0">, </span>
        <span v-if="fileCount > 0">{{ fileCount.toLocaleString() }} file{{ fileCount !== 1 ? 's' : '' }}</span>
      </template>
    </h3>

    <dl class="space-y-2">
      <!-- Name (single only) -->
      <div v-if="items.length === 1" class="flex flex-col sm:flex-row sm:gap-2">
        <dt class="text-xs text-zinc-500 sm:w-20 sm:shrink-0">Name</dt>
        <dd class="text-xs text-zinc-200 break-all">{{ items[0]?.name }}</dd>
      </div>

      <!-- Location -->
      <div class="flex flex-col sm:flex-row sm:gap-2">
        <dt class="text-xs text-zinc-500 sm:w-20 sm:shrink-0">Location</dt>
        <dd class="text-xs text-zinc-400 break-all font-mono">{{ location }}</dd>
      </div>

      <!-- Type (single only) -->
      <div v-if="items.length === 1" class="flex flex-col sm:flex-row sm:gap-2">
        <dt class="text-xs text-zinc-500 sm:w-20 sm:shrink-0">Type</dt>
        <dd class="text-xs text-zinc-200">{{ items[0]?.is_dir ? 'Folder' : (items[0]?.mime || 'Unknown') }}</dd>
      </div>

      <!-- Size -->
      <div class="flex flex-col sm:flex-row sm:gap-2">
        <dt class="text-xs text-zinc-500 sm:w-20 sm:shrink-0">Size</dt>
        <dd class="text-xs text-zinc-200">
          <template v-if="items.length === 1 && !items[0]?.is_dir">
            {{ formatSize(items[0]?.size ?? 0) }}
          </template>
          <template v-else>{{ sizeText }}</template>
        </dd>
      </div>

      <!-- Created (single only, if filesystem provides it) -->
      <div v-if="items.length === 1 && items[0]?.created_at" class="flex flex-col sm:flex-row sm:gap-2">
        <dt class="text-xs text-zinc-500 sm:w-20 sm:shrink-0">Created</dt>
        <dd class="text-xs text-zinc-200">{{ formatDate(items[0]?.created_at ?? '') }}</dd>
      </div>

      <!-- Modified (single only) -->
      <div v-if="items.length === 1" class="flex flex-col sm:flex-row sm:gap-2">
        <dt class="text-xs text-zinc-500 sm:w-20 sm:shrink-0">Modified</dt>
        <dd class="text-xs text-zinc-200">{{ formatDate(items[0]?.mod_time ?? '') }}</dd>
      </div>

      <!-- Image resolution -->
      <div v-if="detailsMedia?.image" class="flex flex-col sm:flex-row sm:gap-2">
        <dt class="text-xs text-zinc-500 sm:w-20 sm:shrink-0">Resolution</dt>
        <dd class="text-xs text-zinc-200">{{ detailsMedia.image.width }} × {{ detailsMedia.image.height }}</dd>
      </div>

      <!-- Video metadata -->
      <template v-if="detailsMedia?.video">
        <div class="flex flex-col sm:flex-row sm:gap-2">
          <dt class="text-xs text-zinc-500 sm:w-20 sm:shrink-0">Duration</dt>
          <dd class="text-xs text-zinc-200">{{ formatDuration(detailsMedia.video.duration) }}</dd>
        </div>
        <div class="flex flex-col sm:flex-row sm:gap-2">
          <dt class="text-xs text-zinc-500 sm:w-20 sm:shrink-0">Resolution</dt>
          <dd class="text-xs text-zinc-200">{{ detailsMedia.video.width }} × {{ detailsMedia.video.height }}</dd>
        </div>
        <div v-if="detailsMedia.video.codec" class="flex flex-col sm:flex-row sm:gap-2">
          <dt class="text-xs text-zinc-500 sm:w-20 sm:shrink-0">Codec</dt>
          <dd class="text-xs text-zinc-200">{{ detailsMedia.video.codec }}</dd>
        </div>
      </template>
    </dl>

    <div class="flex justify-end mt-4">
      <AppButton @click="emit('close')">Close</AppButton>
    </div>
  </AppDialog>
</template>
