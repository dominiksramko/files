<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { marked, Renderer, type Tokens } from 'marked'
import hljs from 'highlight.js/lib/common'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { fileCategory, formatSize, isMarkdownFile, isTextFile } from '@/utils/formatters'

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

// ── MIME helpers ─────────────────────────────────────────────────────────────
const streamUrl  = computed(() => (props.entry ? filesApi.streamUrl(props.entry.path) : ''))
const mime       = computed(() => props.entry?.mime ?? '')
const isImage    = computed(() => mime.value.startsWith('image/'))
const isVideo    = computed(() => mime.value.startsWith('video/'))
const isAudio    = computed(() => mime.value.startsWith('audio/'))
const isPdf      = computed(() => mime.value === 'application/pdf')
const isText     = computed(() => !!props.entry && isTextFile(mime.value, props.entry.name))
const isMarkdown = computed(() => !!props.entry && isMarkdownFile(mime.value, props.entry.name))
const isPreviewable = computed(
  () => isImage.value || isVideo.value || isAudio.value || isPdf.value || isText.value,
)

const category = computed(() =>
  props.entry ? fileCategory(props.entry.mime, props.entry.name) : 'other',
)
const categoryStyle: Record<string, { bg: string; icon: string }> = {
  office: { bg: 'bg-blue-950/60', icon: 'text-blue-400' },
  text:   { bg: 'bg-zinc-800/60', icon: 'text-zinc-300' },
  other:  { bg: 'bg-zinc-800/60', icon: 'text-zinc-500' },
}
const fallbackStyle = { bg: 'bg-zinc-800/60', icon: 'text-zinc-500' }
const catStyle = computed(() => categoryStyle[category.value] ?? fallbackStyle)

// ── Configure marked with hljs code block highlighting ────────────────────────
const renderer = new Renderer()
renderer.code = function ({ text, lang }: Tokens.Code) {
  const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext'
  const highlighted = hljs.highlight(text, { language }).value
  return `<pre class="hljs-md-pre"><code class="hljs language-${language}">${highlighted}</code></pre>`
}
marked.use({ renderer })

// ── Text content fetching ─────────────────────────────────────────────────────
const TEXT_SIZE_LIMIT = 5 * 1024 * 1024 // 5 MB

const textContent  = ref<string | null>(null)
const textLoading  = ref(false)
const textError    = ref(false)
const textTooLarge = ref(false)

let fetchController: AbortController | null = null

watch(
  () => props.entry,
  async (entry) => {
    fetchController?.abort()
    fetchController = null
    textContent.value  = null
    textError.value    = false
    textTooLarge.value = false

    if (!entry || !isText.value) return

    if (entry.size > TEXT_SIZE_LIMIT) {
      textTooLarge.value = true
      return
    }

    textLoading.value = true
    fetchController = new AbortController()
    try {
      const res = await fetch(filesApi.streamUrl(entry.path), { signal: fetchController.signal })
      if (!res.ok) throw new Error()
      textContent.value = await res.text()
    } catch (e) {
      if ((e as Error).name !== 'AbortError') textError.value = true
    } finally {
      textLoading.value = false
    }
  },
  { immediate: true },
)

const renderedMarkdown = computed(() => {
  if (!isMarkdown.value || textContent.value === null) return ''
  return marked.parse(textContent.value) as string
})

function highlightCode(entry: FileEntry | null, raw: string | null): string {
  if (!raw) return ''
  if (!entry) return escapeHtml(raw)
  const ext = entry.name.split('.').pop()?.toLowerCase() ?? ''
  try {
    if (ext && hljs.getLanguage(ext)) {
      return hljs.highlight(raw, { language: ext }).value
    }
    return hljs.highlightAuto(raw).value
  } catch {
    return escapeHtml(raw)
  }
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

// ── Keyboard navigation ───────────────────────────────────────────────────────
function onKey(e: KeyboardEvent) {
  if (!props.entry) return
  if (e.key === 'Escape') emit('close')
  if (e.key === 'ArrowLeft'  && props.hasPrev) emit('prev')
  if (e.key === 'ArrowRight' && props.hasNext) emit('next')
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

          <!-- Media / text content -->
          <div class="flex-1 flex items-center justify-center min-h-0 min-w-0">

            <!-- Image -->
            <img
              v-if="isImage"
              :key="entry.path"
              :src="streamUrl"
              :alt="entry.name"
              class="max-w-full max-h-full object-contain rounded-lg select-none"
              style="max-height: calc(100vh - 120px)"
              draggable="false"
              @click.stop
            />

            <!-- Video -->
            <video
              v-else-if="isVideo"
              :key="entry.path"
              :src="streamUrl"
              controls
              class="max-w-full rounded-lg"
              style="max-height: calc(100vh - 120px)"
              preload="metadata"
              @click.stop
            />

            <!-- Audio -->
            <div v-else-if="isAudio" class="w-full max-w-md px-6" @click.stop>
              <div class="flex items-center gap-5 mb-6">
                <div class="size-20 rounded-2xl bg-pink-950/60 flex items-center justify-center shrink-0">
                  <svg class="size-10 text-pink-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                      d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3" />
                  </svg>
                </div>
                <p class="text-white font-medium text-lg leading-tight">{{ entry.name }}</p>
              </div>
              <audio :key="entry.path" :src="streamUrl" controls class="w-full" />
            </div>

            <!-- PDF -->
            <iframe
              v-else-if="isPdf"
              :key="entry.path"
              :src="streamUrl"
              class="w-full rounded-lg border border-zinc-700"
              style="height: calc(100vh - 120px)"
              title="PDF preview"
            />

            <!-- Text / Markdown / Code -->
            <div
              v-else-if="isText"
              class="flex-1 min-w-0 overflow-y-auto"
              style="max-height: calc(100vh - 120px)"
              @click.stop
            >
              <!-- Loading -->
              <div
                v-if="textLoading"
                class="flex items-center justify-center h-32 text-zinc-400 text-sm"
              >
                Loading…
              </div>

              <!-- Error -->
              <div
                v-else-if="textError"
                class="flex items-center justify-center h-32 text-red-400 text-sm"
              >
                Failed to load file content.
              </div>

              <!-- Too large to preview -->
              <div
                v-else-if="textTooLarge"
                class="flex flex-col items-center justify-center h-32 gap-3 text-zinc-400 text-sm"
              >
                <p>File is too large to preview ({{ formatSize(entry.size) }})</p>
                <a
                  :href="filesApi.downloadUrl(entry.path)"
                  target="_blank"
                  class="text-blue-400 hover:text-blue-300 underline"
                  @click.stop
                >Download to view</a>
              </div>

              <!-- Markdown rendered -->
              <article
                v-else-if="isMarkdown && textContent !== null"
                class="markdown-body px-8 py-6 mx-auto max-w-4xl w-full"
                v-html="renderedMarkdown"
              />

              <!-- Code / plain text with syntax highlighting -->
              <pre
                v-else-if="textContent !== null"
                class="text-sm leading-relaxed p-6"
              ><code class="hljs block rounded-lg" v-html="highlightCode(entry, textContent)" /></pre>
            </div>

            <!-- No preview — file info + download card -->
            <div v-else class="flex flex-col items-center gap-6 p-8 w-full" @click.stop>
              <div :class="['size-24 rounded-2xl flex items-center justify-center', catStyle.bg]">
                <svg
                  :class="['size-12', catStyle.icon]"
                  fill="none" viewBox="0 0 24 24" stroke="currentColor"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
                    d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                </svg>
              </div>
              <div class="text-center space-y-1">
                <p class="text-white font-semibold text-lg leading-tight break-all">{{ entry.name }}</p>
                <p class="text-zinc-400 text-sm">{{ formatSize(entry.size) }}</p>
                <p class="text-zinc-600 text-xs font-mono">{{ entry.mime || 'Unknown type' }}</p>
              </div>
              <a
                :href="filesApi.downloadUrl(entry.path)"
                target="_blank"
                class="flex items-center gap-2 bg-blue-600 hover:bg-blue-700 text-white font-medium text-sm px-6 py-2.5 rounded-xl transition-colors"
                @click.stop
              >
                <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                    d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                </svg>
                Download
              </a>
            </div>
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

<!-- Global: targets v-html rendered markdown and hljs blocks -->
<style>
.markdown-body {
  color: #e4e4e7;
  font-size: 0.9375rem;
  line-height: 1.75;
}
.markdown-body h1,
.markdown-body h2,
.markdown-body h3,
.markdown-body h4,
.markdown-body h5,
.markdown-body h6 {
  color: #fff;
  font-weight: 600;
  margin-top: 1.5em;
  margin-bottom: 0.5em;
  line-height: 1.3;
}
.markdown-body h1 { font-size: 1.875rem; border-bottom: 1px solid #3f3f46; padding-bottom: 0.4em; }
.markdown-body h2 { font-size: 1.5rem;   border-bottom: 1px solid #3f3f46; padding-bottom: 0.3em; }
.markdown-body h3 { font-size: 1.25rem; }
.markdown-body h4 { font-size: 1.0625rem; }
.markdown-body p  { margin-bottom: 1em; }
.markdown-body a  { color: #60a5fa; text-decoration: underline; }
.markdown-body a:hover { color: #93c5fd; }
.markdown-body strong { color: #fff; font-weight: 600; }
.markdown-body em     { font-style: italic; }
.markdown-body ul,
.markdown-body ol  { margin-bottom: 1em; padding-left: 1.75em; }
.markdown-body ul  { list-style-type: disc; }
.markdown-body ol  { list-style-type: decimal; }
.markdown-body li  { margin-bottom: 0.25em; }
.markdown-body li > ul,
.markdown-body li > ol { margin-bottom: 0; }
.markdown-body blockquote {
  border-left: 3px solid #52525b;
  padding: 0.25em 1em;
  margin: 1em 0;
  color: #a1a1aa;
}
.markdown-body code {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 0.875em;
  background: #27272a;
  padding: 0.15em 0.4em;
  border-radius: 0.3em;
  color: #f4f4f5;
}
.markdown-body pre,
.markdown-body .hljs-md-pre {
  margin: 1em 0;
  border-radius: 0.5rem;
  overflow: auto;
}
.markdown-body pre code,
.markdown-body .hljs-md-pre code {
  background: transparent;
  padding: 0;
  font-size: 0.875rem;
  line-height: 1.6;
}
.markdown-body table {
  width: 100%;
  border-collapse: collapse;
  margin-bottom: 1em;
}
.markdown-body th,
.markdown-body td {
  border: 1px solid #3f3f46;
  padding: 0.5em 0.75em;
  text-align: left;
}
.markdown-body th { background: #27272a; color: #fff; font-weight: 600; }
.markdown-body tr:nth-child(even) td { background: #18181b; }
.markdown-body hr {
  border: none;
  border-top: 1px solid #3f3f46;
  margin: 1.5em 0;
}
.markdown-body img { max-width: 100%; border-radius: 0.5rem; }
</style>
