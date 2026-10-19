<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import hljs from 'highlight.js/lib/common'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { useFilesStore } from '@/stores/files'
import { useFileContent } from '@/composables/useFileContent'
import { isMarkdownFile } from '@/utils/formatters'
import { renderMarkdown } from '@/utils/markdown'
import EditableFileState from '@/components/files/EditableFileState.vue'

const props = defineProps<{ entry: FileEntry; initialEdit?: boolean }>()

const filesStore = useFilesStore()

const textContent = ref<string | null>(null)
const { loading: textLoading, loadError: textError, tooLarge: textTooLarge } = useFileContent(
  () => props.entry,
  (content) => {
    textContent.value = content
    if (props.initialEdit) startEdit()
  },
)

// ── Edit mode ─────────────────────────────────────────────────────────────
const editing     = ref(false)
const editContent = ref('')
const saving      = ref(false)
const saveError   = ref('')
const textareaRef = ref<HTMLTextAreaElement | null>(null)

async function startEdit() {
  editContent.value = textContent.value ?? ''
  saveError.value = ''
  editing.value = true
  await nextTick()
  textareaRef.value?.focus()
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

const renderedMarkdown = computed(() => {
  if (!isMarkdown.value || textContent.value === null) return ''
  return renderMarkdown(textContent.value, props.entry.path)
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
      <EditableFileState
        :entry="entry"
        :loading="textLoading"
        :load-error="textError"
        :too-large="textTooLarge"
      >
        <textarea
          v-if="editing"
          ref="textareaRef"
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
      </EditableFileState>
    </div>
  </div>
</template>
