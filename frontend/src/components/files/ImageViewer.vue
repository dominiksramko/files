<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import AppIcon from '@/components/ui/AppIcon.vue'

const props = defineProps<{ entry: FileEntry }>()

const streamUrl = filesApi.streamUrl(props.entry.path, props.entry.mod_time)

const transparentMimes = new Set([
  'image/png', 'image/svg+xml', 'image/webp', 'image/gif', 'image/avif',
  'image/tiff', 'image/jxl', 'image/x-icon', 'image/vnd.adobe.photoshop',
  'image/x-tga', 'image/x-xcf', 'image/x-exr', 'image/vnd.ms-dds',
])
const hasAlpha = transparentMimes.has(props.entry.mime)
const imageError = ref(false)

const zoom           = ref(1)
const panX           = ref(0)
const panY           = ref(0)
const isPanning      = ref(false)
const panOrigin      = ref({ x: 0, y: 0, panX: 0, panY: 0 })
const imageContainer = ref<HTMLElement | null>(null)

function resetZoom() { zoom.value = 1; panX.value = 0; panY.value = 0 }
function zoomIn()    { zoom.value = Math.min(8, zoom.value * 1.25) }
function zoomOut()   {
  zoom.value = Math.max(1, zoom.value / 1.25)
  if (zoom.value === 1) { panX.value = 0; panY.value = 0 }
}
function toggleZoom() {
  if (zoom.value === 1) zoom.value = 2
  else resetZoom()
}

// --- Mouse wheel zoom ---
function onWheel(e: WheelEvent) {
  const factor  = e.deltaY < 0 ? 1.15 : 1 / 1.15
  const newZoom = Math.max(1, Math.min(8, zoom.value * factor))
  if (newZoom === zoom.value) return
  if (imageContainer.value) {
    const rect  = imageContainer.value.getBoundingClientRect()
    const ex    = e.clientX - (rect.left + rect.width  / 2)
    const ey    = e.clientY - (rect.top  + rect.height / 2)
    const ratio = newZoom / zoom.value
    panX.value  = ex * (1 - ratio) + panX.value * ratio
    panY.value  = ey * (1 - ratio) + panY.value * ratio
  }
  zoom.value = newZoom
  if (zoom.value === 1) { panX.value = 0; panY.value = 0 }
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

let touchState: TouchState = null
let lastTapTime = 0

function touchDist(touches: TouchList): number {
  const dx = touches[0].clientX - touches[1].clientX
  const dy = touches[0].clientY - touches[1].clientY
  return Math.sqrt(dx * dx + dy * dy)
}

function onTouchStart(e: TouchEvent) {
  if (e.touches.length === 2) {
    e.preventDefault()
    touchState = {
      type:     'pinch',
      prevDist: touchDist(e.touches),
      prevMidX: (e.touches[0].clientX + e.touches[1].clientX) / 2,
      prevMidY: (e.touches[0].clientY + e.touches[1].clientY) / 2,
    }
  } else if (e.touches.length === 1 && zoom.value > 1) {
    touchState = {
      type:    'pan',
      originX: e.touches[0].clientX - panX.value,
      originY: e.touches[0].clientY - panY.value,
    }
  } else {
    touchState = null
  }
}

function onTouchMove(e: TouchEvent) {
  if (!touchState) return
  e.preventDefault()

  if (touchState.type === 'pinch' && e.touches.length === 2) {
    const newDist = touchDist(e.touches)
    const newMidX = (e.touches[0].clientX + e.touches[1].clientX) / 2
    const newMidY = (e.touches[0].clientY + e.touches[1].clientY) / 2
    const newZoom = Math.max(1, Math.min(8, zoom.value * (newDist / touchState.prevDist)))
    const ratio   = newZoom / zoom.value

    if (imageContainer.value) {
      const rect   = imageContainer.value.getBoundingClientRect()
      const pinchX = touchState.prevMidX - (rect.left + rect.width  / 2)
      const pinchY = touchState.prevMidY - (rect.top  + rect.height / 2)
      // Zoom around the pinch midpoint, then shift by midpoint translation
      panX.value = pinchX * (1 - ratio) + panX.value * ratio + (newMidX - touchState.prevMidX)
      panY.value = pinchY * (1 - ratio) + panY.value * ratio + (newMidY - touchState.prevMidY)
    }

    zoom.value = newZoom
    if (zoom.value === 1) { panX.value = 0; panY.value = 0 }
    touchState.prevDist = newDist
    touchState.prevMidX = newMidX
    touchState.prevMidY = newMidY

  } else if (touchState.type === 'pan' && e.touches.length === 1) {
    panX.value = e.touches[0].clientX - touchState.originX
    panY.value = e.touches[0].clientY - touchState.originY
  }
}

function onTouchEnd(e: TouchEvent) {
  // Finger lifted during pinch → switch to pan with remaining finger
  if (e.touches.length === 1 && touchState?.type === 'pinch') {
    touchState = {
      type:    'pan',
      originX: e.touches[0].clientX - panX.value,
      originY: e.touches[0].clientY - panY.value,
    }
    return
  }

  if (e.touches.length > 0) return
  touchState = null

  // Double-tap detection — only for single-finger taps on the image area
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
  if (e.key === '+' || e.key === '=') { e.preventDefault(); zoomIn() }
  if (e.key === '-')                  { e.preventDefault(); zoomOut() }
  if (e.key === '0')                  { e.preventDefault(); resetZoom() }
}

onMounted(()  => window.addEventListener('keydown', onKey))
onUnmounted(() => {
  window.removeEventListener('keydown',   onKey)
  window.removeEventListener('mousemove', onPanMove)
  window.removeEventListener('mouseup',   onPanEnd)
})
</script>

<template>
  <div
    class="self-stretch flex flex-col min-h-0 min-w-0 select-none py-4"
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
      class="flex-1 flex items-center justify-center min-h-0 min-w-0"
      @dblclick="toggleZoom"
    >
      <div
        v-if="imageError"
        class="flex flex-col items-center justify-center gap-3 text-zinc-500"
      >
        <AppIcon name="xCircle" class="size-12" :stroke-width="1.5" />
        <p class="text-sm">Preview not available — download to view</p>
      </div>
      <img
        v-else
        :src="streamUrl"
        :alt="entry.name"
        :style="{
          transform: `translate(${panX}px, ${panY}px) scale(${zoom})`,
          transformOrigin: 'center center',
          transition: isPanning || touchState ? 'none' : 'transform 0.1s ease',
          willChange: 'transform',
        }"
        :class="['max-h-full object-contain select-none', hasAlpha && 'checkerboard']"
        draggable="false"
        @error="imageError = true"
      />
    </div>

    <!-- Zoom controls — always below the image, never overlapping -->
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
