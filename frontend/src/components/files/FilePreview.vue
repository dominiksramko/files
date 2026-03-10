<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { isOfficeFile, isTextFile } from '@/utils/formatters'
import AudioPlayer  from '@/components/files/AudioPlayer.vue'
import FileFallback from '@/components/files/FileFallback.vue'
import ImageViewer  from '@/components/files/ImageViewer.vue'
import OfficeViewer from '@/components/files/OfficeViewer.vue'
import PdfViewer    from '@/components/files/PdfViewer.vue'
import TextViewer   from '@/components/files/TextViewer.vue'
import VideoPlayer  from '@/components/files/VideoPlayer.vue'
import AppIcon      from '@/components/ui/AppIcon.vue'

const props = defineProps<{
  entry: FileEntry | null
  hasPrev: boolean
  hasNext: boolean
  index: number
  total: number
  textEditMode?: boolean
}>()

const emit = defineEmits<{
  close: []
  prev: []
  next: []
  rename: [entry: FileEntry]
  delete: [entry: FileEntry]
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
  if (isOfficeFile(m, e.name)) return OfficeViewer
  return FileFallback
})

function onKey(e: KeyboardEvent) {
  if (!props.entry) return
  const tag = (document.activeElement as HTMLElement)?.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA') return
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
              :href="filesApi.downloadUrl(entry.path, entry.mod_time)"
              target="_blank"
              class="text-zinc-400 hover:text-white transition-colors p-1.5 rounded-lg hover:bg-white/10"
              title="Download"
              @click.stop
            >
              <AppIcon name="download" class="size-5" />
            </a>
            <button
              class="text-zinc-400 hover:text-white transition-colors p-1.5 rounded-lg hover:bg-white/10"
              title="Rename"
              @click.stop="emit('rename', entry)"
            >
              <AppIcon name="pencilSquare" class="size-5" />
            </button>
            <button
              class="text-zinc-400 hover:text-red-400 transition-colors p-1.5 rounded-lg hover:bg-white/10"
              title="Delete"
              @click.stop="emit('delete', entry)"
            >
              <AppIcon name="trash" class="size-5" />
            </button>
            <button
              class="text-zinc-400 hover:text-white transition-colors p-1.5 rounded-lg hover:bg-white/10"
              @click="emit('close')"
            >
              <AppIcon name="x" class="size-5" />
            </button>
          </div>
        </div>

        <!-- Main content + side arrows -->
        <div class="flex-1 flex items-stretch min-h-0 gap-2 px-2 overflow-hidden">
          <!-- Prev arrow -->
          <button
            :class="[
              'self-center relative z-10 shrink-0 p-2 rounded-full transition-all',
              hasPrev
                ? 'text-white bg-white/10 hover:bg-white/20 cursor-pointer'
                : 'text-transparent pointer-events-none',
            ]"
            @click.stop="emit('prev')"
          >
            <AppIcon name="chevronLeft" class="size-7" />
          </button>

          <!-- Dynamic viewer -->
          <div class="flex-1 flex items-center justify-center min-h-0 min-w-0" @click.stop>
            <component
              :is="viewerComponent"
              v-if="viewerComponent"
              :key="entry.path"
              :entry="entry"
              v-bind="viewerComponent === TextViewer ? { initialEdit: textEditMode } : {}"
            />
          </div>

          <!-- Next arrow -->
          <button
            :class="[
              'self-center relative z-10 shrink-0 p-2 rounded-full transition-all',
              hasNext
                ? 'text-white bg-white/10 hover:bg-white/20 cursor-pointer'
                : 'text-transparent pointer-events-none',
            ]"
            @click.stop="emit('next')"
          >
            <AppIcon name="chevronRight" class="size-7" />
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
