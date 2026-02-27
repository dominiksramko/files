<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { isTextFile } from '@/utils/formatters'
import AudioPlayer  from '@/components/files/AudioPlayer.vue'
import FileFallback from '@/components/files/FileFallback.vue'
import ImageViewer  from '@/components/files/ImageViewer.vue'
import PdfViewer    from '@/components/files/PdfViewer.vue'
import TextViewer   from '@/components/files/TextViewer.vue'
import VideoPlayer  from '@/components/files/VideoPlayer.vue'

const props = defineProps<{
  entry: FileEntry | null
  hasPrev: boolean
  hasNext: boolean
  index: number
  total: number
}>()

const emit = defineEmits<{
  close: []
  prev: []
  next: []
}>()

const viewerComponent = computed(() => {
  const e = props.entry
  if (!e) return null
  const m = e.mime
  if (m.startsWith('image/')) return ImageViewer
  if (m.startsWith('video/')) return VideoPlayer
  if (m.startsWith('audio/')) return AudioPlayer
  if (m === 'application/pdf') return PdfViewer
  if (isTextFile(m, e.name)) return TextViewer
  return FileFallback
})

function onKey(e: KeyboardEvent) {
  if (!props.entry) return
  if (e.key === 'Escape')                       emit('close')
  if (e.key === 'ArrowLeft'  && props.hasPrev)  emit('prev')
  if (e.key === 'ArrowRight' && props.hasNext)  emit('next')
}

onMounted(()  => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <Teleport to="body">
    <Transition name="lightbox">
      <div
        v-if="entry"
        class="fixed inset-0 z-50 flex flex-col bg-black/90 backdrop-blur-sm"
        @click="emit('close')"
      >
        <!-- Top bar -->
        <div class="flex items-center justify-between px-4 py-3 shrink-0" @click.stop>
          <span class="text-sm text-white font-medium truncate max-w-xs">{{ entry.name }}</span>
          <div class="flex items-center gap-3 shrink-0 ml-4">
            <span class="text-xs text-zinc-400 tabular-nums">{{ index + 1 }} / {{ total }}</span>
            <a
              :href="filesApi.downloadUrl(entry.path)"
              target="_blank"
              class="text-zinc-400 hover:text-white transition-colors p-1.5 rounded-lg hover:bg-white/10"
              title="Download"
              @click.stop
            >
              <svg class="size-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
            </a>
            <button
              class="text-zinc-400 hover:text-white transition-colors p-1.5 rounded-lg hover:bg-white/10"
              @click="emit('close')"
            >
              <svg class="size-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                  d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        <!-- Main content + side arrows -->
        <div class="flex-1 flex items-center min-h-0 gap-2 px-2">
          <!-- Prev arrow -->
          <button
            :class="[
              'shrink-0 p-2 rounded-full transition-all',
              hasPrev
                ? 'text-white bg-white/10 hover:bg-white/20 cursor-pointer'
                : 'text-transparent pointer-events-none',
            ]"
            @click.stop="emit('prev')"
          >
            <svg class="size-7" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
            </svg>
          </button>

          <!-- Dynamic viewer -->
          <div class="flex-1 flex items-center justify-center min-h-0 min-w-0" @click.stop>
            <component
              :is="viewerComponent"
              v-if="viewerComponent"
              :key="entry.path"
              :entry="entry"
            />
          </div>

          <!-- Next arrow -->
          <button
            :class="[
              'shrink-0 p-2 rounded-full transition-all',
              hasNext
                ? 'text-white bg-white/10 hover:bg-white/20 cursor-pointer'
                : 'text-transparent pointer-events-none',
            ]"
            @click.stop="emit('next')"
          >
            <svg class="size-7" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
            </svg>
          </button>
        </div>

        <!-- Bottom padding -->
        <div class="shrink-0 h-4" />
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.lightbox-enter-active,
.lightbox-leave-active {
  transition: opacity 0.15s ease;
}
.lightbox-enter-from,
.lightbox-leave-to {
  opacity: 0;
}
</style>
