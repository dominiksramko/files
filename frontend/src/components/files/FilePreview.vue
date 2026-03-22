<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
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
  prevEntry?: FileEntry | null
  nextEntry?: FileEntry | null
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

function viewerFor(entry: FileEntry) {
  const mime = entry.mime
  if (mime.startsWith('image/')) return ImageViewer
  if (mime.startsWith('video/')) return VideoPlayer
  if (mime.startsWith('audio/')) return AudioPlayer
  if (mime === 'application/pdf') return PdfViewer
  if (isTextFile(mime, entry.name)) return TextViewer
  if (isOfficeFile(mime, entry.name)) return OfficeViewer
  return FileFallback
}

function extraProps(entry: FileEntry, active: boolean): Record<string, unknown> {
  const viewer = viewerFor(entry)
  const result: Record<string, unknown> = {}
  if (viewer === VideoPlayer) result.active = active
  if (viewer === TextViewer && active) result.initialEdit = props.textEditMode
  return result
}

// --- Carousel ---
const carouselRef = ref<HTMLElement | null>(null)
const trackRef    = ref<HTMLElement | null>(null)

// Always 3 slots: [prev, current, next]. Null for missing boundary entries.
const panelEntries = computed<(FileEntry | null)[]>(() => [
  props.prevEntry ?? null,
  props.entry,
  props.nextEntry ?? null,
])

const dragX             = ref(0)
const transitionEnabled = ref(false)
const isSnapping        = ref(false)

let dragStartX    = 0
let dragStartY    = 0
let dragStartTime = 0
let dragDirection: 'h' | 'v' | null = null
let isSwiping     = false

function onCarouselTouchStart(e: TouchEvent) {
  if (isSnapping.value || e.touches.length !== 1) return
  dragStartX    = e.touches[0]!.clientX
  dragStartY    = e.touches[0]!.clientY
  dragStartTime = Date.now()
  dragDirection = null
  isSwiping     = true
  transitionEnabled.value = false
}

function onCarouselTouchMove(e: TouchEvent) {
  if (!isSwiping || e.touches.length !== 1) return
  const deltaX = e.touches[0]!.clientX - dragStartX
  const deltaY = e.touches[0]!.clientY - dragStartY

  if (!dragDirection) {
    if (Math.abs(deltaX) > 6 || Math.abs(deltaY) > 6) {
      dragDirection = Math.abs(deltaX) > Math.abs(deltaY) ? 'h' : 'v'
      // If horizontal but image is zoomed, let ImageViewer handle it as a pan
      if (dragDirection === 'h' && trackRef.value?.children[1]?.querySelector('[data-image-zoomed]')) {
        dragDirection = null
        isSwiping = false
      }
    }
    return
  }

  if (dragDirection !== 'h') return
  e.preventDefault()

  // Resistance at boundaries
  let dx = deltaX
  if (dx > 0 && !props.hasPrev) dx *= 0.15
  if (dx < 0 && !props.hasNext) dx *= 0.15
  dragX.value = dx
}

function onCarouselTouchEnd() {
  if (!isSwiping) return
  isSwiping = false

  if (dragDirection !== 'h') { dragDirection = null; return }
  dragDirection = null

  const containerW = carouselRef.value?.clientWidth ?? window.innerWidth
  const elapsed    = Math.max(1, Date.now() - dragStartTime)
  const velocity   = dragX.value / elapsed // px/ms

  const shouldSnap = Math.abs(dragX.value) > containerW * 0.35 || Math.abs(velocity) > 0.3

  if (shouldSnap && dragX.value < 0 && props.hasNext) snapTo('next')
  else if (shouldSnap && dragX.value > 0 && props.hasPrev) snapTo('prev')
  else snapTo('reset')
}

function snapTo(direction: 'prev' | 'next' | 'reset') {
  transitionEnabled.value = true
  isSnapping.value        = true

  if (direction === 'reset') {
    dragX.value = 0
    setTimeout(() => {
      transitionEnabled.value = false
      isSnapping.value        = false
    }, 300)
    return
  }

  const containerW = carouselRef.value?.clientWidth ?? window.innerWidth
  dragX.value = direction === 'prev' ? containerW : -containerW

  setTimeout(() => {
    transitionEnabled.value = false
    if (direction === 'prev') emit('prev')
    else emit('next')
    dragX.value      = 0
    isSnapping.value = false
  }, 300)
}

// --- Keyboard ---
function onKey(e: KeyboardEvent) {
  if (!props.entry) return
  const tag = (document.activeElement as HTMLElement)?.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA') return
  if (e.key === 'Escape') { emit('close'); return }
  const currentPanel = trackRef.value?.children[1] as HTMLElement | undefined
  const mediaEl = currentPanel?.querySelector<HTMLVideoElement | HTMLAudioElement>('video, audio')
  if (mediaEl && !mediaEl.paused) {
    if (e.key === 'ArrowLeft')  { e.preventDefault(); mediaEl.currentTime = Math.max(0, mediaEl.currentTime - 5) }
    if (e.key === 'ArrowRight') { e.preventDefault(); mediaEl.currentTime = Math.min(mediaEl.duration, mediaEl.currentTime + 5) }
    return
  }
  if (e.key === 'ArrowLeft'  && props.hasPrev) snapTo('prev')
  if (e.key === 'ArrowRight' && props.hasNext) snapTo('next')
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

        <!-- Carousel -->
        <div
          ref="carouselRef"
          class="flex-1 relative min-h-0 overflow-hidden"
          @touchstart.passive="onCarouselTouchStart"
          @touchmove="onCarouselTouchMove"
          @touchend="onCarouselTouchEnd"
        >
          <!-- Track: 3 panels side by side.
               Key is on the panel div so Vue moves entire panels (preserving mounted
               component state) instead of unmounting/remounting on navigation. -->
          <div
            ref="trackRef"
            class="flex h-full"
            :style="{
              width: '300%',
              transform: `translateX(calc(-33.333% + ${dragX}px))`,
              transition: transitionEnabled ? 'transform 0.3s cubic-bezier(0.25, 0.46, 0.45, 0.94)' : 'none',
              willChange: 'transform',
            }"
          >
            <div
              v-for="(panelEntry, slotIndex) in panelEntries"
              :key="panelEntry?.path ?? (slotIndex === 0 ? 'slot-prev' : 'slot-next')"
              class="h-full overflow-hidden flex items-center justify-center"
              style="width: 33.333%"
              @click.stop
            >
              <component
                v-if="panelEntry"
                :is="viewerFor(panelEntry)"
                :entry="panelEntry"
                v-bind="extraProps(panelEntry, panelEntry === entry)"
              />
            </div>
          </div>

          <!-- Prev arrow (overlaid) -->
          <button
            :class="[
              'absolute left-2 top-1/2 -translate-y-1/2 z-10 p-2 rounded-full transition-all',
              hasPrev
                ? 'text-white bg-white/10 hover:bg-white/20 cursor-pointer'
                : 'text-transparent pointer-events-none',
            ]"
            @click.stop="snapTo('prev')"
          >
            <AppIcon name="chevronLeft" class="size-7" />
          </button>

          <!-- Next arrow (overlaid) -->
          <button
            :class="[
              'absolute right-2 top-1/2 -translate-y-1/2 z-10 p-2 rounded-full transition-all',
              hasNext
                ? 'text-white bg-white/10 hover:bg-white/20 cursor-pointer'
                : 'text-transparent pointer-events-none',
            ]"
            @click.stop="snapTo('next')"
          >
            <AppIcon name="chevronRight" class="size-7" />
          </button>
        </div>
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
