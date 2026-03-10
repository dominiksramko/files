<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { marked, Renderer, type Tokens } from 'marked'
import hljs from 'highlight.js/lib/common'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { useFilesStore } from '@/stores/files'
import { formatSize, isMarkdownFile } from '@/utils/formatters'

const props = defineProps<{ entry: FileEntry }>()

const filesStore = useFilesStore()

const TEXT_SIZE_LIMIT = 2 * 1024 * 1024 // 2 MB

const textContent  = ref<string | null>(null)
const textLoading  = ref(false)
const textError    = ref(false)
const textTooLarge = ref(false)

// ── Edit mode ─────────────────────────────────────────────────────────────
const editing     = ref(false)
const editContent = ref('')
const saving      = ref(false)
const saveError   = ref('')

function startEdit() {
  editContent.value = textContent.value ?? ''
  saveError.value = ''
  editing.value = true
}

function cancelEdit() {
  editing.value = false
  saveError.value = ''
}

async function save() {
  saving.value = true
  saveError.value = ''
  try {
    await filesApi.writeContent(props.entry.path, editContent.value)
    textContent.value = editContent.value
    editing.value = false
    filesStore.refresh()
  } catch (e) {
    saveError.value = (e as Error).message
  } finally {
    saving.value = false
  }
}

// ── Markdown + syntax highlighting ────────────────────────────────────────
const isMarkdown = computed(() => isMarkdownFile(props.entry.mime, props.entry.name))

const renderer = new Renderer()
renderer.code = function ({ text, lang }: Tokens.Code) {
  const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext'
  const highlighted = hljs.highlight(text, { language }).value
  return `<pre class="hljs-md-pre"><code class="hljs language-${language}">${highlighted}</code></pre>`
}
marked.use({ renderer })

const renderedMarkdown = computed(() => {
  if (!isMarkdown.value || textContent.value === null) return ''
  return marked.parse(textContent.value) as string
})

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function highlightCode(raw: string | null): string {
  if (!raw) return ''
  const ext = props.entry.name.split('.').pop()?.toLowerCase() ?? ''
  try {
    if (ext && hljs.getLanguage(ext)) {
      return hljs.highlight(raw, { language: ext }).value
    }
    return hljs.highlightAuto(raw).value
  } catch {
    return escapeHtml(raw)
  }
}

// ── Fetch ─────────────────────────────────────────────────────────────────
let fetchController: AbortController | null = null

onMounted(async () => {
  if (props.entry.size > TEXT_SIZE_LIMIT) {
    textTooLarge.value = true
    return
  }

  textLoading.value = true
  fetchController = new AbortController()
  try {
    const res = await fetch(filesApi.streamUrl(props.entry.path, props.entry.mod_time), { signal: fetchController.signal })
    if (!res.ok) throw new Error()
    textContent.value = await res.text()
  } catch (e) {
    if ((e as Error).name !== 'AbortError') textError.value = true
  } finally {
    textLoading.value = false
  }
})

onBeforeUnmount(() => {
  fetchController?.abort()
})
</script>

<template>
  <div
    class="self-stretch w-full min-w-0 flex flex-col"
    @click.stop
  >
    <!-- Toolbar: only shown when content is ready -->
    <div
      v-if="textContent !== null && !textTooLarge"
      class="shrink-0 flex items-center flex-wrap gap-x-2 gap-y-1 px-4 py-1.5 border-b border-zinc-800/60"
      @click.stop
    >
      <template v-if="editing">
        <button
          :disabled="saving"
          class="px-3 py-1.5 text-xs bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white rounded-lg transition-colors"
          @click="save"
        >{{ saving ? 'Saving…' : 'Save' }}</button>
        <button
          class="px-3 py-1.5 text-xs text-zinc-400 hover:text-white rounded-lg hover:bg-zinc-800 transition-colors"
          @click="cancelEdit"
        >Cancel</button>
      </template>
      <button
        v-else
        class="px-3 py-1.5 text-xs text-zinc-400 hover:text-white rounded-lg hover:bg-zinc-800 transition-colors"
        @click="startEdit"
      >Edit</button>
      <span v-if="saveError" class="text-xs text-red-400">{{ saveError }}</span>
    </div>

    <!-- Content area -->
    <div class="flex-1 min-h-0 flex flex-col">
      <div v-if="textLoading" class="flex items-center justify-center h-32 text-zinc-400 text-sm">
        Loading…
      </div>
      <div v-else-if="textError" class="flex items-center justify-center h-32 text-red-400 text-sm">
        Failed to load file content.
      </div>
      <div v-else-if="textTooLarge" class="flex flex-col items-center justify-center h-32 gap-3 text-zinc-400 text-sm">
        <p>File is too large to preview ({{ formatSize(entry.size) }})</p>
        <a
          :href="filesApi.downloadUrl(entry.path, entry.mod_time)"
          target="_blank"
          class="text-blue-400 hover:text-blue-300 underline"
          @click.stop
        >Download to view</a>
      </div>
      <textarea
        v-else-if="editing"
        v-model="editContent"
        class="flex-1 w-full bg-zinc-900 text-zinc-100 font-mono text-sm leading-relaxed p-6 resize-none focus:outline-none"
        spellcheck="false"
      />
      <article
        v-else-if="isMarkdown && textContent !== null"
        class="flex-1 overflow-y-auto min-h-0 markdown-body px-8 py-6 w-full bg-zinc-900"
        v-html="renderedMarkdown"
      />
      <pre
        v-else-if="textContent !== null"
        class="flex-1 overflow-auto min-h-0 text-sm leading-relaxed p-6 bg-zinc-900"
      ><code class="hljs block" v-html="highlightCode(textContent)" /></pre>
    </div>
  </div>
</template>

<!-- Global: targets v-html rendered markdown and hljs blocks -->
<style>
/* Let the <pre> background fill the full height; theme bg only on code blocks */
.hljs { background: transparent !important; }
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
