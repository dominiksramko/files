<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import AppIcon from '@/components/ui/AppIcon.vue'
import { isModalOpen, isTypingTarget } from '@/utils/keyboard'
import { fitScale as viewerScale, placeholderSize, type Size } from '@/utils/imageFit'

// active: whether this is the image on screen (the preview also keeps its
// neighbours mounted); only that one takes zoom keys.
const props = withDefaults(defineProps<{ entry: FileEntry; active?: boolean }>(), { active: true })

const streamUrl = filesApi.streamUrl(props.entry.path, props.entry.mod_time)

const transparentMimes = new Set([
  'image/png', 'image/svg+xml', 'image/webp', 'image/gif', 'image/avif',
  'image/tiff', 'image/jxl', 'image/x-icon', 'image/vnd.adobe.photoshop',
  'image/x-tga', 'image/x-xcf', 'image/x-exr', 'image/vnd.ms-dds',
])
// Shown behind transparent images once loaded — before, the browser already
// sizes the image from its first bytes, and the background would hide the
// placeholder while the rest arrives.
const hasAlpha   = transparentMimes.has(props.entry.mime)
const imageError = ref(false)
const imgLoaded  = ref(false)
// Whether the loaded image has been put in place; until then it moves there
// without the zoom transition, which would slide it in from the top left
// (it sat centred by a transform relative to its own size while loading).
const imgPlaced  = ref(false)
const naturalW   = ref(0)
const naturalH   = ref(0)
const containerW = ref(0)
const containerH = ref(0)

const zoom       = ref(1)
const panX       = ref(0)
const panY       = ref(0)
const isPanning  = ref(false)
const isWheeling = ref(false)
const panOrigin  = ref({ x: 0, y: 0, panX: 0, panY: 0 })
let wheelTimer: ReturnType<typeof setTimeout> | null = null
const imageContainer = ref<HTMLElement | null>(null)

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value))
}

// Clamp pan so the image edge never moves past the container edge.
function clampPan() {
  const halfOverflowX = Math.max(0, ((displayW.value ?? 0) - containerW.value) / 2)
  const halfOverflowY = Math.max(0, ((displayH.value ?? 0) - containerH.value) / 2)
  panX.value = clamp(panX.value, -halfOverflowX, halfOverflowX)
  panY.value = clamp(panY.value, -halfOverflowY, halfOverflowY)
}

const containerSize = computed(() => ({ width: containerW.value, height: containerH.value }))

// Scale that makes the image fill the container at zoom=1 (same as object-contain)
const fitScale = computed(() => viewerScale({ width: naturalW.value, height: naturalH.value }, containerSize.value))

// The file's small whole-picture thumbnail, shown where the image will be
// until it has loaded. Computed so it appears if the thumbnail is made while
// the preview is open.
const placeholderUrl = computed(() =>
  props.entry.has_thumb && props.entry.thumb_hash
    ? filesApi.previewThumbUrl(props.entry.path, props.entry.thumb_hash)
    : null,
)
const placeholderNatural = ref<Size | null>(null)
const placeholderFailed  = ref(false)
const placeholderStyle = computed(() => {
  if (!placeholderNatural.value) return { visibility: 'hidden' as const }
  const size = placeholderSize(placeholderNatural.value, containerSize.value)
  return {
    left: 0,
    top: 0,
    width: `${size.width}px`,
    height: `${size.height}px`,
    maxWidth: 'none',
    maxHeight: 'none',
    transform: `translate(${Math.round((containerW.value - size.width) / 2)}px, ${Math.round((containerH.value - size.height) / 2)}px)`,
  }
})

function onPlaceholderLoad(event: Event) {
  const placeholder = event.target as HTMLImageElement
  placeholderNatural.value = { width: placeholder.naturalWidth, height: placeholder.naturalHeight }
}

// Explicit CSS dimensions instead of transform scale.
// The browser renders the image at exactly these pixel dimensions — always
// downsampling from native resolution → sharp at any zoom up to the image's own size.
const displayW = computed(() => imgLoaded.value ? fitScale.value * naturalW.value * zoom.value : null)
const displayH = computed(() => imgLoaded.value ? fitScale.value * naturalH.value * zoom.value : null)

// Pixel-integer translate values so the image snaps to whole pixels — prevents
// the checkerboard background bleeding around sub-pixel edges.
const imgTranslateX = computed(() =>
  displayW.value != null
    ? Math.round((containerW.value - displayW.value) / 2 + panX.value)
    : 0,
)
const imgTranslateY = computed(() =>
  displayH.value != null
    ? Math.round((containerH.value - displayH.value) / 2 + panY.value)
    : 0,
)

async function onImageLoad(e: Event) {
  const img = e.target as HTMLImageElement
  naturalW.value = img.naturalWidth
  naturalH.value = img.naturalHeight
  if (imageContainer.value) {
    containerW.value = imageContainer.value.clientWidth
    containerH.value = imageContainer.value.clientHeight
  }
  imgLoaded.value = true
  // Reading its size makes the browser apply the new place before the
  // transition is turned on.
  await nextTick()
  img.getBoundingClientRect()
  imgPlaced.value = true
}

function resetZoom() { zoom.value = 1; panX.value = 0; panY.value = 0 }
function zoomIn()    { zoom.value = Math.min(8, zoom.value * 1.25); clampPan() }
function zoomOut()   {
  zoom.value = Math.max(1, zoom.value / 1.25)
  if (zoom.value === 1) { panX.value = 0; panY.value = 0 }
  else clampPan()
}
function toggleZoom() {
  if (zoom.value === 1) zoom.value = 2
  else resetZoom()
}

// Shift pan so the point (ex, ey) relative to container center stays fixed after zoom
function zoomToward(ex: number, ey: number, newZoom: number) {
  const ratio = newZoom / zoom.value
  panX.value  = ex * (1 - ratio) + panX.value * ratio
  panY.value  = ey * (1 - ratio) + panY.value * ratio
  zoom.value  = newZoom
  if (zoom.value === 1) { panX.value = 0; panY.value = 0 }
}

// --- Mouse wheel zoom ---
function onWheel(e: WheelEvent) {
  isWheeling.value = true
  if (wheelTimer) clearTimeout(wheelTimer)
  wheelTimer = setTimeout(() => { isWheeling.value = false }, 150)
  const factor  = e.deltaY < 0 ? 1.08 : 1 / 1.08
  const newZoom = Math.max(1, Math.min(8, zoom.value * factor))
  if (newZoom === zoom.value) return
  if (imageContainer.value) {
    const rect = imageContainer.value.getBoundingClientRect()
    zoomToward(e.clientX - (rect.left + rect.width / 2), e.clientY - (rect.top + rect.height / 2), newZoom)
  } else {
    zoom.value = newZoom
  }
  clampPan()
}

// --- Mouse pan ---
function onPanStart(e: MouseEvent) {
  if (zoom.value <= 1) return
  e.preventDefault()
  isPanning.value = true
  panOrigin.value = { x: e.clientX, y: e.clientY, panX: panX.value, panY: panY.value }
  window.addEventListener('mousemove', onPanMove)
  window.addEventListener('mouseup',   onPanEnd)
}

function onPanMove(e: MouseEvent) {
  panX.value = panOrigin.value.panX + (e.clientX - panOrigin.value.x)
  panY.value = panOrigin.value.panY + (e.clientY - panOrigin.value.y)
  clampPan()
}

function onPanEnd() {
  isPanning.value = false
  window.removeEventListener('mousemove', onPanMove)
  window.removeEventListener('mouseup',   onPanEnd)
}

// --- Touch: pinch-to-zoom + single-finger pan + double-tap ---
interface TouchPinchState { type: 'pinch'; prevDist: number; prevMidX: number; prevMidY: number }
interface TouchPanState   { type: 'pan';   originX: number;  originY: number }
type TouchState = TouchPinchState | TouchPanState | null

const touchState = ref<TouchState>(null)
let lastTapTime = 0

function touchDist(touches: TouchList): number {
  const t0 = touches[0]!, t1 = touches[1]!
  return Math.sqrt((t0.clientX - t1.clientX) ** 2 + (t0.clientY - t1.clientY) ** 2)
}

function touchMid(touches: TouchList): { x: number; y: number } {
  const t0 = touches[0]!, t1 = touches[1]!
  return { x: (t0.clientX + t1.clientX) / 2, y: (t0.clientY + t1.clientY) / 2 }
}

function onTouchStart(e: TouchEvent) {
  if (e.touches.length === 2) {
    e.preventDefault()
    const mid = touchMid(e.touches)
    touchState.value = { type: 'pinch', prevDist: touchDist(e.touches), prevMidX: mid.x, prevMidY: mid.y }
  } else if (e.touches.length === 1 && zoom.value > 1) {
    const t = e.touches[0]!
    touchState.value = { type: 'pan', originX: t.clientX - panX.value, originY: t.clientY - panY.value }
  } else {
    touchState.value = null
  }
}

function onTouchMove(e: TouchEvent) {
  if (!touchState.value) return
  e.preventDefault()

  if (touchState.value.type === 'pinch' && e.touches.length === 2) {
    const newDist = touchDist(e.touches)
    const newMid  = touchMid(e.touches)
    const newZoom = Math.max(1, Math.min(8, zoom.value * (newDist / touchState.value.prevDist)))

    if (imageContainer.value) {
      const rect   = imageContainer.value.getBoundingClientRect()
      const pinchX = touchState.value.prevMidX - (rect.left + rect.width  / 2)
      const pinchY = touchState.value.prevMidY - (rect.top  + rect.height / 2)
      zoomToward(pinchX, pinchY, newZoom)
      panX.value += newMid.x - touchState.value.prevMidX
      panY.value += newMid.y - touchState.value.prevMidY
    }

    clampPan()
    touchState.value.prevDist = newDist
    touchState.value.prevMidX = newMid.x
    touchState.value.prevMidY = newMid.y

  } else if (touchState.value.type === 'pan' && e.touches.length === 1) {
    const t = e.touches[0]!
    panX.value = t.clientX - touchState.value.originX
    panY.value = t.clientY - touchState.value.originY
    clampPan()
  }
}

function onTouchEnd(e: TouchEvent) {
  if (e.touches.length === 1 && touchState.value?.type === 'pinch') {
    const t = e.touches[0]!
    touchState.value = { type: 'pan', originX: t.clientX - panX.value, originY: t.clientY - panY.value }
    return
  }

  if (e.touches.length > 0) return
  touchState.value = null

  if (e.changedTouches.length !== 1) return
  const now = Date.now()
  if (now - lastTapTime < 300) {
    e.preventDefault()
    toggleZoom()
    lastTapTime = 0
  } else {
    lastTapTime = now
  }
}

// --- Keyboard ---
function onKey(e: KeyboardEvent) {
  // Ctrl/Cmd with - and = is the browser's own page zoom.
  if (!props.active || isTypingTarget(e.target) || isModalOpen() || e.ctrlKey || e.metaKey || e.altKey) return
  if (e.key === '+' || e.key === '=') { e.preventDefault(); zoomIn() }
  if (e.key === '-')                  { e.preventDefault(); zoomOut() }
  if (e.key === '0')                  { e.preventDefault(); resetZoom() }
}

let resizeObserver: ResizeObserver | null = null

onMounted(() => {
  window.addEventListener('keydown', onKey)
  if (imageContainer.value) {
    resizeObserver = new ResizeObserver(entries => {
      const rect = entries[0]?.contentRect
      if (rect) { containerW.value = rect.width; containerH.value = rect.height }
    })
    resizeObserver.observe(imageContainer.value)
  }
})

onUnmounted(() => {
  window.removeEventListener('keydown',   onKey)
  window.removeEventListener('mousemove', onPanMove)
  window.removeEventListener('mouseup',   onPanEnd)
  resizeObserver?.disconnect()
  if (wheelTimer) clearTimeout(wheelTimer)
})
</script>

<template>
  <div
    class="self-stretch w-full flex flex-col min-h-0 min-w-0 select-none py-4"
    :data-image-zoomed="zoom > 1 ? true : undefined"
    :style="{ cursor: zoom > 1 ? (isPanning ? 'grabbing' : 'grab') : 'default' }"
    @click.stop
    @wheel.prevent="onWheel"
    @mousedown="onPanStart"
    @touchstart.passive="onTouchStart"
    @touchmove.prevent="onTouchMove"
    @touchend="onTouchEnd"
  >
    <!-- Image area -->
    <div
      ref="imageContainer"
      class="flex-1 w-full relative overflow-hidden min-h-0"
      @dblclick="toggleZoom"
    >
      <img
        v-if="placeholderUrl && !placeholderFailed && !imgLoaded && !imageError"
        :src="placeholderUrl"
        alt=""
        aria-hidden="true"
        :style="placeholderStyle"
        class="absolute block select-none pointer-events-none"
        draggable="false"
        @load="onPlaceholderLoad"
        @error="placeholderFailed = true"
      />

      <div
        v-if="imageError"
        class="absolute inset-0 flex flex-col items-center justify-center gap-3 text-zinc-500"
      >
        <AppIcon name="xCircle" class="size-12" :stroke-width="1.5" />
        <p class="text-sm">Preview not available — download to view</p>
      </div>

      <img
        v-else
        :src="streamUrl"
        :alt="entry.name"
        :style="displayW ? {
          position: 'absolute',
          left: 0,
          top: 0,
          width: `${displayW}px`,
          height: `${displayH}px`,
          maxWidth: 'none',
          maxHeight: 'none',
          transform: `translate(${imgTranslateX}px, ${imgTranslateY}px)`,
          transition: !imgPlaced || touchState || isPanning || isWheeling ? 'none' : 'transform 0.1s ease',
        } : {
          position: 'absolute',
          left: '50%',
          top: '50%',
          maxWidth: '100%',
          maxHeight: '100%',
          transform: 'translate(-50%, -50%)',
        }"
        :class="['block select-none', hasAlpha && imgLoaded && 'checkerboard']"
        draggable="false"
        @load="onImageLoad"
        @error="imageError = true"
      />
    </div>

    <!-- Zoom controls -->
    <div v-if="!imageError" class="relative z-10 shrink-0 flex justify-center py-2">
      <div class="flex items-center gap-0.5 bg-black/60 backdrop-blur-sm rounded-full px-2 py-1">
        <button
          :class="['p-1 rounded-full transition-colors', zoom <= 1 ? 'text-zinc-600 cursor-default' : 'text-zinc-300 hover:text-white hover:bg-white/10']"
          title="Zoom out (-)"
          @click.stop="zoomOut"
          @touchend.stop
        >
          <AppIcon name="zoomOut" class="size-4" />
        </button>
        <button
          class="text-xs text-zinc-300 tabular-nums w-10 text-center select-none hover:text-white transition-colors rounded px-0.5 py-1"
          title="Reset zoom (0)"
          @click.stop="resetZoom"
          @touchend.stop
        >{{ Math.round(zoom * 100) }}%</button>
        <button
          :class="['p-1 rounded-full transition-colors', zoom >= 8 ? 'text-zinc-600 cursor-default' : 'text-zinc-300 hover:text-white hover:bg-white/10']"
          title="Zoom in (+)"
          @click.stop="zoomIn"
          @touchend.stop
        >
          <AppIcon name="zoomIn" class="size-4" />
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.checkerboard {
  background-color: #fff;
  background-image:
    linear-gradient(45deg, #ccc 25%, transparent 25%),
    linear-gradient(-45deg, #ccc 25%, transparent 25%),
    linear-gradient(45deg, transparent 75%, #ccc 75%),
    linear-gradient(-45deg, transparent 75%, #ccc 75%);
  background-size: 16px 16px;
  background-position: 0 0, 0 8px, 8px -8px, -8px 0px;
}
</style>
