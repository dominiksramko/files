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
    class="self-stretch flex flex-col min-h-0 min-w-0 overflow-hidden select-none py-4"
    :style="{ cursor: zoom > 1 ? (isPanning ? 'grabbing' : 'grab') : 'default' }"
    @click.stop
    @dblclick.stop="resetZoom"
    @wheel.prevent="onWheel"
    @mousedown="onPanStart"
  >
    <!-- Image area -->
    <div
      ref="imageContainer"
      class="flex-1 flex items-center justify-center min-h-0 min-w-0"
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
          transition: isPanning ? 'none' : 'transform 0.1s ease',
          willChange: 'transform',
        }"
        :class="['w-full h-full object-contain rounded-lg select-none', hasAlpha && 'checkerboard']"
        draggable="false"
        @error="imageError = true"
      />
    </div>

    <!-- Zoom controls — always below the image, never overlapping -->
    <div v-if="!imageError" class="shrink-0 flex justify-center py-2">
      <div class="flex items-center gap-0.5 bg-black/60 backdrop-blur-sm rounded-full px-2 py-1">
        <button
          :class="['p-1 rounded-full transition-colors', zoom <= 1 ? 'text-zinc-600 cursor-default' : 'text-zinc-300 hover:text-white hover:bg-white/10']"
          title="Zoom out (-)"
          @click.stop="zoomOut"
        >
          <AppIcon name="zoomOut" class="size-4" />
        </button>
        <button
          class="text-xs text-zinc-300 tabular-nums w-10 text-center select-none hover:text-white transition-colors rounded px-0.5 py-1"
          title="Reset zoom (0)"
          @click.stop="resetZoom"
        >{{ Math.round(zoom * 100) }}%</button>
        <button
          :class="['p-1 rounded-full transition-colors', zoom >= 8 ? 'text-zinc-600 cursor-default' : 'text-zinc-300 hover:text-white hover:bg-white/10']"
          title="Zoom in (+)"
          @click.stop="zoomIn"
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
