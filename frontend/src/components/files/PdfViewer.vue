<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'

const props = defineProps<{ entry: FileEntry }>()

const streamUrl = filesApi.streamUrl(props.entry.path, props.entry.mod_time)

const useNative = typeof navigator !== 'undefined' && !!navigator.pdfViewerEnabled

// --- PDF.js continuous scroll renderer ---
const scrollContainerRef = ref<HTMLElement | null>(null)
const pageRefs           = ref<(HTMLCanvasElement | null)[]>([])
const totalPages         = ref(0)
const loading            = ref(false)
const errorMsg           = ref('')

// eslint-disable-next-line @typescript-eslint/no-explicit-any
let pdfDoc: any = null
let alive = true
const activeRenderTasks = new Map<number, { cancel(): void }>()

async function loadPdf() {
  loading.value = true
  errorMsg.value = ''
  try {
    const pdfjsLib = await import('pdfjs-dist')
    pdfjsLib.GlobalWorkerOptions.workerSrc = new URL(
      'pdfjs-dist/build/pdf.worker.min.mjs',
      import.meta.url,
    ).toString()

    const response = await fetch(streamUrl)
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    const arrayBuffer = await response.arrayBuffer()

    if (!alive) return
    pdfDoc = await pdfjsLib.getDocument({ data: arrayBuffer }).promise
    if (!alive) return
    totalPages.value = pdfDoc.numPages
  } catch (err) {
    if (!alive) return
    errorMsg.value = err instanceof Error ? err.message : 'Failed to load PDF'
  } finally {
    if (alive) loading.value = false
  }
}

async function renderPageOnCanvas(pageNum: number, canvas: HTMLCanvasElement) {
  // Cancel any in-progress render for this page
  activeRenderTasks.get(pageNum)?.cancel()
  activeRenderTasks.delete(pageNum)

  const page         = await pdfDoc.getPage(pageNum)
  const containerW   = scrollContainerRef.value?.clientWidth ?? canvas.parentElement?.clientWidth ?? 600
  const naturalViewport = page.getViewport({ scale: 1 })
  const scale        = (containerW / naturalViewport.width) * window.devicePixelRatio
  const viewport     = page.getViewport({ scale })

  canvas.width        = viewport.width
  canvas.height       = viewport.height
  canvas.style.width  = '100%'
  canvas.style.height = 'auto'

  const task = page.render({ canvasContext: canvas.getContext('2d')!, viewport })
  activeRenderTasks.set(pageNum, task)
  try {
    await task.promise
  } catch {
    // render cancelled — ignore
  }
  activeRenderTasks.delete(pageNum)
}

let intersectionObserver: IntersectionObserver | null = null
let resizeObserver: ResizeObserver | null = null
const renderedPages = new Set<number>()

function observePages() {
  intersectionObserver?.disconnect()
  intersectionObserver = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue
        const canvas  = entry.target as HTMLCanvasElement
        const pageNum = parseInt(canvas.dataset.page!)
        if (!renderedPages.has(pageNum)) {
          renderedPages.add(pageNum)
          renderPageOnCanvas(pageNum, canvas)
        }
      }
    },
    { root: scrollContainerRef.value, rootMargin: '300px' },
  )
  for (const canvas of pageRefs.value) {
    if (canvas) intersectionObserver.observe(canvas)
  }
}

function rerenderAll() {
  renderedPages.clear()
  for (const [, task] of activeRenderTasks) task.cancel()
  activeRenderTasks.clear()
  observePages()
  // Re-render immediately-visible pages
  for (const canvas of pageRefs.value) {
    if (!canvas) continue
    const rect = canvas.getBoundingClientRect()
    if (rect.top < window.innerHeight + 300) {
      const pageNum = parseInt(canvas.dataset.page!)
      renderedPages.add(pageNum)
      renderPageOnCanvas(pageNum, canvas)
    }
  }
}

onMounted(async () => {
  if (useNative) return
  await loadPdf()
  if (!alive || !pdfDoc) return

  await nextTick() // let v-for render the canvas placeholders

  observePages()

  resizeObserver = new ResizeObserver(rerenderAll)
  if (scrollContainerRef.value) resizeObserver.observe(scrollContainerRef.value)
})

onUnmounted(() => {
  alive = false
  for (const [, task] of activeRenderTasks) task.cancel()
  activeRenderTasks.clear()
  intersectionObserver?.disconnect()
  resizeObserver?.disconnect()
  pdfDoc?.destroy()
})
</script>

<template>
  <!-- Native inline renderer (desktop) -->
  <div v-if="useNative" class="w-full h-full" @click.stop>
    <iframe
      :src="streamUrl"
      class="w-full h-full rounded-lg border border-zinc-700"
      title="PDF preview"
    />
  </div>

  <!-- PDF.js continuous scroll renderer -->
  <div v-else class="w-full h-full overflow-y-auto" ref="scrollContainerRef" @click.stop>
    <!-- Loading -->
    <div v-if="loading" class="flex items-center justify-center gap-2 text-zinc-400 py-16">
      <AppSpinner size="sm" />
      <span class="text-sm">Loading PDF…</span>
    </div>

    <!-- Error -->
    <div v-else-if="errorMsg" class="flex flex-col items-center justify-center gap-2 text-zinc-500 py-16">
      <AppIcon name="xCircle" class="size-10" :stroke-width="1.5" />
      <p class="text-sm">{{ errorMsg }}</p>
    </div>

    <!-- Pages stacked vertically -->
    <div v-else-if="pdfDoc" class="flex flex-col gap-2 py-4 px-4">
      <canvas
        v-for="pageNum in totalPages"
        :key="pageNum"
        :ref="el => pageRefs[pageNum - 1] = el as HTMLCanvasElement | null"
        :data-page="pageNum"
        class="w-full rounded shadow-lg bg-white"
      />
    </div>
  </div>
</template>
