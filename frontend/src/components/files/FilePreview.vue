<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { catIconName, customViewName, fileCategory, isKanbanFile, isOfficeFile, isTextFile, isTodoFile } from '@/utils/formatters'
import AudioPlayer  from '@/components/files/AudioPlayer.vue'
import FileFallback from '@/components/files/FileFallback.vue'
import ImageViewer  from '@/components/files/ImageViewer.vue'
import KanbanViewer from '@/components/files/KanbanViewer.vue'
import OfficeViewer from '@/components/files/OfficeViewer.vue'
import PdfViewer    from '@/components/files/PdfViewer.vue'
import TextViewer   from '@/components/files/TextViewer.vue'
import TodoViewer   from '@/components/files/TodoViewer.vue'
import VideoPlayer  from '@/components/files/VideoPlayer.vue'
import AppIcon      from '@/components/ui/AppIcon.vue'
import { isTypingTarget } from '@/utils/keyboard'

const props = defineProps<{
  entry: FileEntry | null
  prevEntry?: FileEntry | null
  nextEntry?: FileEntry | null
  hasPrev: boolean
  hasNext: boolean
  index: number
  total: number
  textEditMode?: boolean
  rawText?: boolean // show the current file in the text viewer instead of its custom view
}>()

const emit = defineEmits<{
  close: []
  prev: []
  next: []
  rename: [entry: FileEntry]
  delete: [entry: FileEntry]
  toggleRawText: []
}>()

// Name of the current file's custom view ("board"), if it has one — enables the raw-text toggle.
const currentViewName = computed(() => (props.entry ? customViewName(props.entry.name) : null))

// Viewers that fetch the whole file or hand it to an outside service: only the
// panel on screen gets one, so merely being next to the open file costs nothing.
const activeOnlyViewers = new Set([OfficeViewer, PdfViewer])

// active: whether entry is the current panel (raw mode only applies to it).
function viewerFor(entry: FileEntry, active: boolean) {
  const viewer = chooseViewer(entry, active)
  return active || !activeOnlyViewers.has(viewer) ? viewer : FileFallback
}

function chooseViewer(entry: FileEntry, active: boolean) {
  const mime = entry.mime
  if (active && props.rawText && customViewName(entry.name)) return TextViewer
  if (mime.startsWith('image/')) return ImageViewer
  if (mime.startsWith('video/')) return VideoPlayer
  if (mime.startsWith('audio/')) return AudioPlayer
  if (mime === 'application/pdf') return PdfViewer
  if (isTodoFile(entry.name)) return TodoViewer
  if (isKanbanFile(entry.name)) return KanbanViewer
  if (isTextFile(mime, entry.name)) return TextViewer
  if (isOfficeFile(mime, entry.name)) return OfficeViewer
  return FileFallback
}

function extraProps(entry: FileEntry, active: boolean): Record<string, unknown> {
  const viewer = viewerFor(entry, active)
  const result: Record<string, unknown> = {}
  if (viewer === VideoPlayer || viewer === ImageViewer) result.active = active
  if (viewer === TodoViewer || viewer === KanbanViewer) result.active = active
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

// Viewers whose content scrolls or drags horizontally (e.g. a kanban board) mark
// that region with data-no-carousel-swipe; gestures starting there aren't navigation.
// Checked on the target instead of stopping propagation, because drag-and-drop
// libraries listen for touchmove on the document.
function ownsHorizontalGestures(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest('[data-no-carousel-swipe]') !== null
}

function onCarouselTouchStart(e: TouchEvent) {
  if (isSnapping.value || e.touches.length !== 1) return
  if (ownsHorizontalGestures(e.target)) {
    isSwiping = false
    return
  }
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

// --- Horizontal wheel / trackpad swipe navigation ---
// deltaX-dominant wheel events (trackpad two-finger horizontal swipe) navigate
// prev/next. A small accumulator prevents accidental triggers from light brushes.
let wheelAccX     = 0
let wheelResetTimer: ReturnType<typeof setTimeout> | null = null
const WHEEL_THRESHOLD = 60 // px of accumulated deltaX before snapping

function onCarouselWheel(e: WheelEvent) {
  // Only act when horizontal scroll is dominant (trackpad swipe, not zoom scroll)
  if (Math.abs(e.deltaX) <= Math.abs(e.deltaY)) return
  if (ownsHorizontalGestures(e.target)) return
  // Don't intercept when image is zoomed — ImageViewer handles panning
  if (trackRef.value?.children[1]?.querySelector('[data-image-zoomed]')) return
  if (isSnapping.value) return

  e.preventDefault()
  wheelAccX += e.deltaX

  if (wheelResetTimer) clearTimeout(wheelResetTimer)
  wheelResetTimer = setTimeout(() => { wheelAccX = 0 }, 300)

  if (wheelAccX > WHEEL_THRESHOLD && props.hasNext) {
    wheelAccX = 0
    snapTo('next')
  } else if (wheelAccX < -WHEEL_THRESHOLD && props.hasPrev) {
    wheelAccX = 0
    snapTo('prev')
  }
}

// --- Keyboard ---
function onKey(e: KeyboardEvent) {
  if (!props.entry) return
  if (isTypingTarget(e.target)) return
  // A modal opened from inside a viewer (e.g. a kanban card) owns the keyboard.
  if (document.querySelector('[aria-modal="true"]')) return
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
onUnmounted(() => {
  window.removeEventListener('keydown', onKey)
  if (wheelResetTimer) clearTimeout(wheelResetTimer)
})
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
            <button
              v-if="currentViewName"
              class="text-zinc-400 hover:text-white transition-colors p-1.5 rounded-lg hover:bg-white/10"
              :title="rawText ? `View as ${currentViewName}` : 'View as text'"
              @click.stop="emit('toggleRawText')"
            >
              <AppIcon :name="rawText ? catIconName[fileCategory(entry.mime, entry.name)] : 'code'" class="size-5" />
            </button>
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
          @wheel="onCarouselWheel"
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
                :is="viewerFor(panelEntry, panelEntry === entry)"
                :entry="panelEntry"
                v-bind="extraProps(panelEntry, panelEntry === entry)"
              />
            </div>
          </div>

          <!-- Prev arrow (overlaid, desktop only — mobile uses swipe) -->
          <button
            :class="[
              'absolute left-2 top-1/2 -translate-y-1/2 z-10 p-2 rounded-full transition-all hidden sm:block',
              hasPrev
                ? 'text-white bg-white/10 hover:bg-white/20 cursor-pointer'
                : 'text-transparent pointer-events-none',
            ]"
            @click.stop="snapTo('prev')"
          >
            <AppIcon name="chevronLeft" class="size-7" />
          </button>

          <!-- Next arrow (overlaid, desktop only — mobile uses swipe) -->
          <button
            :class="[
              'absolute right-2 top-1/2 -translate-y-1/2 z-10 p-2 rounded-full transition-all hidden sm:block',
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
