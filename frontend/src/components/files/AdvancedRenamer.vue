<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { debounce } from '@/utils/debounce'
import type { FileEntry, RenamePreset, RenamePreviewItem } from '@/types'
import { renamer, ConflictError } from '@/utils/api'
import { useToast } from '@/composables/useToast'
import { useConflictDialog } from '@/composables/useConflictDialog'
import AppModal from '@/components/ui/AppModal.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import PresetPill from '@/components/files/PresetPill.vue'
import { escapeHtml } from '@/utils/html'

const props = defineProps<{
  files: FileEntry[]
  folder: string
}>()

const emit = defineEmits<{
  close: []
}>()

const { show: showToast } = useToast()
const { prompt: promptConflict } = useConflictDialog()

// ── Variable / transform definitions ─────────────────────────────────────────

const VARIABLE_CATEGORIES = [
  {
    key: 'file' as const,
    label: 'File',
    color: '#4ade80',
    vars: [
      { name: 'name', desc: 'Filename without extension', example: 'IMG_4872' },
      { name: 'ext', desc: 'Extension without dot', example: 'jpg' },
      { name: 'index', desc: 'Batch counter (use with pad:3)', example: '1' },
      { name: 'size', desc: 'File size in KB', example: '2048' },
      { name: 'random', desc: 'Random 6-char hex', example: 'a3f9c1' },
      { name: 'created_date', desc: 'Creation date YYYY-MM-DD', example: '2024-03-15' },
      { name: 'created_time', desc: 'Creation time HH-MM-SS', example: '14-30-22' },
      { name: 'modified_date', desc: 'Modified date YYYY-MM-DD', example: '2024-11-01' },
      { name: 'modified_time', desc: 'Modified time HH-MM-SS', example: '09-05-47' },
    ],
  },
  {
    key: 'image' as const,
    label: 'Image',
    color: '#60a5fa',
    vars: [
      { name: 'width', desc: 'Width in pixels', example: '4032' },
      { name: 'height', desc: 'Height in pixels', example: '3024' },
      { name: 'megapixels', desc: 'Megapixels (1 decimal)', example: '12.2' },
      { name: 'camera', desc: 'Camera model (EXIF)', example: 'iPhone_15_Pro' },
      { name: 'iso', desc: 'ISO value (EXIF)', example: '800' },
      { name: 'aperture', desc: 'Aperture (EXIF)', example: 'f2.8' },
      { name: 'shutter', desc: 'Shutter speed (EXIF)', example: '1_125s' },
    ],
  },
  {
    key: 'video' as const,
    label: 'Video',
    color: '#f472b6',
    vars: [
      { name: 'duration', desc: 'Duration HH-MM-SS', example: '01-23-45' },
      { name: 'fps', desc: 'Frames per second', example: '24' },
      { name: 'resolution', desc: 'Resolution shorthand', example: '1080p' },
      { name: 'vcodec', desc: 'Video codec', example: 'H264' },
    ],
  },
  {
    key: 'audio' as const,
    label: 'Audio',
    color: '#fb923c',
    vars: [
      { name: 'artist', desc: 'Artist tag', example: 'Pink Floyd' },
      { name: 'album', desc: 'Album tag', example: 'Dark Side of the Moon' },
      { name: 'title', desc: 'Track title tag', example: 'Money' },
      { name: 'track', desc: 'Track number', example: '6' },
      { name: 'year', desc: 'Release year', example: '1973' },
    ],
  },
]

const TRANSFORMS = [
  { syntax: 'upper', desc: 'UPPERCASE' },
  { syntax: 'lower', desc: 'lowercase' },
  { syntax: 'title', desc: 'Title Case' },
  { syntax: 'snake', desc: 'snake_case' },
  { syntax: 'kebab', desc: 'kebab-case' },
  { syntax: 'slug', desc: 'url-safe-slug' },
  { syntax: 'trim', desc: 'Trim whitespace' },
  { syntax: 'pad:N', desc: 'Zero-pad to N digits — e.g. {index|pad:3} → 007' },
  { syntax: 'trunc:N', desc: 'Truncate to N characters' },
  { syntax: 'replace:a:b', desc: 'Replace a with b — e.g. {name|replace: :_}' },
  { syntax: 'prepend:text', desc: 'Add prefix' },
  { syntax: 'append:text', desc: 'Add suffix' },
  { syntax: 'initials', desc: 'First letter of each word' },
  { syntax: 'digits', desc: 'Keep digits only' },
  { syntax: 'alpha', desc: 'Keep letters only' },
  { syntax: 'sep:char', desc: 'Replace separators — e.g. sep:- or sep:_' },
]

// ── Template & highlighted preview ───────────────────────────────────────────

const DEFAULT_TEMPLATE = '{name}.{ext}'
const template = ref(DEFAULT_TEMPLATE)
const editorRef = ref<HTMLDivElement | null>(null)
const savedCaretOffset = ref(0)


function getCaretCharOffset(el: HTMLElement): number {
  const selection = window.getSelection()
  if (!selection || !selection.rangeCount) return 0
  const range = selection.getRangeAt(0)
  const preRange = document.createRange()
  preRange.selectNodeContents(el)
  preRange.setEnd(range.endContainer, range.endOffset)
  return preRange.toString().length
}

function setCaretCharOffset(el: HTMLElement, offset: number): void {
  const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT)
  let lastTextNode: Text | null = null
  let nextNode: Node | null
  while ((nextNode = walker.nextNode())) {
    const textNode = nextNode as Text
    lastTextNode = textNode
    if (offset <= textNode.length) {
      const range = document.createRange()
      range.setStart(textNode, offset)
      range.collapse(true)
      const selection = window.getSelection()
      if (selection) { selection.removeAllRanges(); selection.addRange(range) }
      return
    }
    offset -= textNode.length
  }
  // offset beyond all text — place caret at end
  if (lastTextNode) {
    const range = document.createRange()
    range.setStart(lastTextNode, lastTextNode.length)
    range.collapse(true)
    const selection = window.getSelection()
    if (selection) { selection.removeAllRanges(); selection.addRange(range) }
  }
}

function renderEditorContent(): void {
  const el = editorRef.value
  if (!el) return
  el.innerHTML = templateParts.value
    .map((part) => {
      if (part.type === 'text') return escapeHtml(part.value)
      return `<span style="color:${part.color};background:${part.color}1a;border:1px solid ${part.color}40;border-radius:2px;padding:0 2px;margin:0 1px">${escapeHtml(part.value)}</span>`
    })
    .join('')
}

function onEditorInput(): void {
  const el = editorRef.value
  if (!el) return
  const newCaret = getCaretCharOffset(el)
  pushUndo(template.value, caretBeforeInput)
  template.value = el.textContent ?? ''
  nextTick(() => {
    renderEditorContent()
    setCaretCharOffset(el, newCaret)
  })
}

function onEditorPaste(event: ClipboardEvent): void {
  event.preventDefault()
  const pastedText = event.clipboardData?.getData('text/plain') ?? ''
  const el = editorRef.value
  if (!el) return
  const caretOffset = getCaretCharOffset(el)
  pushUndo(template.value, caretOffset)
  const currentText = el.textContent ?? ''
  const newCaretOffset = caretOffset + pastedText.length
  template.value = currentText.slice(0, caretOffset) + pastedText + currentText.slice(caretOffset)
  nextTick(() => {
    renderEditorContent()
    setCaretCharOffset(el, newCaretOffset)
  })
}

function onEditorBlur(): void {
  const el = editorRef.value
  if (el) savedCaretOffset.value = getCaretCharOffset(el)
}

// ── Undo / redo stack ─────────────────────────────────────────────────────────

interface EditorSnapshot { text: string; caret: number }

const undoStack: EditorSnapshot[] = []
const redoStack: EditorSnapshot[] = []
let caretBeforeInput = 0

function pushUndo(text: string, caret: number) {
  undoStack.push({ text, caret })
  if (undoStack.length > 200) undoStack.shift()
  redoStack.length = 0
}

function applySnapshot(snapshot: EditorSnapshot) {
  const el = editorRef.value
  if (!el) return
  template.value = snapshot.text
  nextTick(() => {
    renderEditorContent()
    setCaretCharOffset(el, snapshot.caret)
  })
}

function undo() {
  if (!undoStack.length) return
  const el = editorRef.value
  if (!el) return
  redoStack.push({ text: template.value, caret: getCaretCharOffset(el) })
  applySnapshot(undoStack.pop()!)
}

function redo() {
  if (!redoStack.length) return
  const el = editorRef.value
  if (!el) return
  undoStack.push({ text: template.value, caret: getCaretCharOffset(el) })
  applySnapshot(redoStack.pop()!)
}

function onEditorKeydown(event: KeyboardEvent) {
  const ctrl = event.ctrlKey || event.metaKey
  if (event.key === 'Enter') { event.preventDefault(); return }
  if (ctrl && event.key === 'z') { event.preventDefault(); undo(); return }
  if (ctrl && (event.key === 'y' || (event.shiftKey && event.key === 'Z'))) {
    event.preventDefault(); redo(); return
  }
  const el = editorRef.value
  if (el) caretBeforeInput = getCaretCharOffset(el)
}

const ALL_VAR_NAMES = new Set(VARIABLE_CATEGORIES.flatMap((c) => c.vars.map((v) => v.name)))

const VAR_COLOR_MAP = new Map<string, string>()
for (const cat of VARIABLE_CATEGORIES) {
  for (const v of cat.vars) {
    VAR_COLOR_MAP.set(v.name, cat.color)
  }
}

interface TemplatePart {
  type: 'text' | 'var'
  value: string
  color: string
  valid: boolean
}

const templateParts = computed((): TemplatePart[] => {
  const parts: TemplatePart[] = []
  const re = /\{([^}]*)\}/g
  let lastIndex = 0
  let match: RegExpExecArray | null
  while ((match = re.exec(template.value)) !== null) {
    if (match.index > lastIndex) {
      parts.push({ type: 'text', value: template.value.slice(lastIndex, match.index), color: '', valid: true })
    }
    const varName = match[1]!.split('|')[0]!.trim()
    const isValid = ALL_VAR_NAMES.has(varName)
    parts.push({
      type: 'var',
      value: match[0],
      color: isValid ? (VAR_COLOR_MAP.get(varName) ?? '#ffffff') : '#f87171',
      valid: isValid,
    })
    lastIndex = match.index + match[0].length
  }
  if (lastIndex < template.value.length) {
    parts.push({ type: 'text', value: template.value.slice(lastIndex), color: '', valid: true })
  }
  return parts
})

// ── Variable picker ───────────────────────────────────────────────────────────

type CategoryKey = (typeof VARIABLE_CATEGORIES)[number]['key']
const activeCategory = ref<CategoryKey>('file')
const showVarPicker = ref(false)
const showTransformsPopover = ref(false)
const showPresetsPopover = ref(false)
const varPickerTriggerRef = ref<HTMLElement | null>(null)
const transformsTriggerRef = ref<HTMLElement | null>(null)
const presetsTriggerRef = ref<HTMLElement | null>(null)
// p-5 = 20px — matches the padding on AppModal's overflow-y-auto content div
const MODAL_CONTENT_PADDING = 20

interface PopoverPos { top: number; left: number; contentLeft: number; contentRight: number }

const varPickerPos = ref<PopoverPos>({ top: 0, left: 0, contentLeft: 0, contentRight: 0 })
const transformsPos = ref<PopoverPos>({ top: 0, left: 0, contentLeft: 0, contentRight: 0 })
const presetsPos = ref<PopoverPos>({ top: 0, left: 0, contentLeft: 0, contentRight: 0 })

function openPopover(
  triggerRef: typeof varPickerTriggerRef,
  posRef: typeof varPickerPos,
  showRef: typeof showVarPicker,
  others: typeof showVarPicker[],
) {
  const el = triggerRef.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  const cardRect = (el.closest('.rounded-xl') as HTMLElement | null)?.getBoundingClientRect()
    ?? { left: 0, right: window.innerWidth }
  posRef.value = {
    top: rect.bottom + 6,
    left: rect.left,
    contentLeft: cardRect.left + MODAL_CONTENT_PADDING,
    contentRight: cardRect.right - MODAL_CONTENT_PADDING,
  }
  const next = !showRef.value
  others.forEach((r) => (r.value = false))
  showRef.value = next
}

function openVarPicker() {
  openPopover(varPickerTriggerRef, varPickerPos, showVarPicker, [showTransformsPopover, showPresetsPopover])
}

function openTransformsPopover() {
  openPopover(transformsTriggerRef, transformsPos, showTransformsPopover, [showVarPicker, showPresetsPopover])
}

function openPresetsPopover() {
  openPopover(presetsTriggerRef, presetsPos, showPresetsPopover, [showVarPicker, showTransformsPopover])
}

function closeAllPopovers() {
  showVarPicker.value = false
  showTransformsPopover.value = false
  showPresetsPopover.value = false
}

onUnmounted(() => {
  document.removeEventListener('click', closeAllPopovers)
  debouncedFetchPreview.cancel()
})

const activeCategoryData = computed(
  () => VARIABLE_CATEGORIES.find((c) => c.key === activeCategory.value)!,
)

function insertVariable(varName: string): void {
  const el = editorRef.value
  if (!el) return
  const insertion = `{${varName}}`
  const caretOffset = document.activeElement === el ? getCaretCharOffset(el) : savedCaretOffset.value
  const currentText = el.textContent ?? ''
  pushUndo(template.value, caretOffset)
  template.value = currentText.slice(0, caretOffset) + insertion + currentText.slice(caretOffset)
  nextTick(() => {
    renderEditorContent()
    el.focus()
    setCaretCharOffset(el, caretOffset + insertion.length)
    showVarPicker.value = false
  })
}

// ── Presets ───────────────────────────────────────────────────────────────────

const presets = ref<RenamePreset[]>([])
const showSavePreset = ref(false)
const savePresetName = ref('')
const savePresetInputRef = ref<HTMLInputElement | null>(null)

watch(showSavePreset, (visible) => {
  if (visible) nextTick(() => savePresetInputRef.value?.focus())
})

async function loadPresets() {
  try {
    presets.value = await renamer.listPresets()
  } catch {
    // non-fatal — user just won't see saved presets
  }
}

async function confirmSavePreset() {
  const name = savePresetName.value.trim()
  if (!name) return
  try {
    const saved = await renamer.savePreset(name, template.value)
    const existingIndex = presets.value.findIndex((p) => p.name === name)
    if (existingIndex >= 0) {
      presets.value[existingIndex] = saved
    } else {
      presets.value.push(saved)
    }
    savePresetName.value = ''
    showSavePreset.value = false
    showToast('Preset saved', 'success')
  } catch (error) {
    showToast((error as Error).message, 'error')
  }
}

async function deletePreset(presetId: number) {
  try {
    await renamer.deletePreset(presetId)
    presets.value = presets.value.filter((p) => p.id !== presetId)
  } catch (error) {
    showToast((error as Error).message, 'error')
  }
}

// ── Preview ───────────────────────────────────────────────────────────────────

const previewItems = ref<RenamePreviewItem[]>([])
const loadingPreview = ref(false)
const debouncedFetchPreview = debounce(fetchPreview, 400)

watch(template, () => {
  debouncedFetchPreview()
  // Re-render editor colors when template changes from outside (presets, folder load).
  // Double-check inside the tick: insertVariable focuses the editor in its own nextTick,
  // which runs before this one — if it already focused+rendered, skip to avoid clobbering the caret.
  if (document.activeElement !== editorRef.value) {
    nextTick(() => {
      if (document.activeElement !== editorRef.value) renderEditorContent()
    })
  }
})

async function fetchPreview() {
  if (!props.files.length || !template.value.trim()) {
    previewItems.value = []
    return
  }
  loadingPreview.value = true
  try {
    const paths = props.files.map((f) => f.path)
    const result = await renamer.preview(template.value, paths)
    previewItems.value = result.items
  } catch (error) {
    showToast((error as Error).message, 'error')
  } finally {
    loadingPreview.value = false
  }
}

// ── Apply ─────────────────────────────────────────────────────────────────────

const applying = ref(false)

const canApply = computed(
  () =>
    previewItems.value.length > 0 &&
    previewItems.value.every((item) => !item.error) &&
    previewItems.value.some((item) => item.new_name !== item.original_name),
)

async function applyRenames(conflictMode?: 'replace' | 'rename') {
  applying.value = true
  try {
    const renames = props.files.map((file, index) => ({
      from: file.path,
      to: previewItems.value[index]?.new_name ?? file.name,
    }))
    await renamer.apply(renames, props.folder, template.value, conflictMode)
    showToast(`Renamed ${renames.length} file${renames.length !== 1 ? 's' : ''}`, 'success')
    emit('close')
  } catch (error) {
    if (error instanceof ConflictError) {
      const choice = await promptConflict('Rename conflicts', error.conflicts)
      if (choice) applyRenames(choice)
      return
    }
    showToast((error as Error).message, 'error')
  } finally {
    applying.value = false
  }
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

// ── Lifecycle ─────────────────────────────────────────────────────────────────

onMounted(() => document.addEventListener('click', closeAllPopovers))

onMounted(async () => {
  await Promise.all([
    loadPresets(),
    renamer.getFolderTemplate(props.folder).then((result) => {
      if (result.template) template.value = result.template
    }).catch(() => {}),
  ])
  renderEditorContent()
  fetchPreview()
})
</script>

<template>
  <AppModal title="Rename (advanced)" size="xl" @close="emit('close')">
    <div class="flex flex-col gap-3">

      <!-- Template input -->
      <div>
        <div class="flex items-center justify-between gap-2 mb-2">
          <span class="text-xs font-medium text-zinc-400 uppercase tracking-wider">Template</span>
          <button
            ref="presetsTriggerRef"
            type="button"
            class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-zinc-800 border border-zinc-700 hover:border-zinc-500 text-xs text-zinc-300 hover:text-zinc-100 transition-colors"
            :class="showPresetsPopover ? 'border-zinc-500 text-zinc-100' : ''"
            @click.stop="openPresetsPopover"
          >
            <AppIcon name="bookmark" class="size-3.5" />
            Presets
            <span v-if="presets.length" class="ml-0.5 bg-zinc-600 text-zinc-300 rounded-full px-1.5 py-px text-[10px] leading-none">{{ presets.length }}</span>
          </button>
        </div>

        <div
          ref="editorRef"
          contenteditable="true"
          spellcheck="false"
          class="w-full bg-zinc-800 rounded-lg px-3 py-2 text-sm font-mono border border-transparent focus:border-zinc-600 focus:outline-none transition-colors whitespace-nowrap overflow-x-auto text-zinc-100 min-h-[2.25rem] leading-relaxed"
          @click.stop
          @input="onEditorInput"
          @paste="onEditorPaste"
          @keydown="onEditorKeydown"
          @blur="onEditorBlur"
        />

        <!-- Action buttons -->
        <div class="mt-2 flex items-center gap-2">
          <button
            ref="varPickerTriggerRef"
            type="button"
            class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-zinc-800 border border-zinc-700 hover:border-zinc-500 text-xs text-zinc-300 hover:text-zinc-100 transition-colors font-mono"
            :class="showVarPicker ? 'border-zinc-500 text-zinc-100' : ''"
            @click.stop="openVarPicker"
          >{…} Variables</button>
          <button
            ref="transformsTriggerRef"
            type="button"
            class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-zinc-800 border border-zinc-700 hover:border-zinc-500 text-xs text-zinc-300 hover:text-zinc-100 transition-colors font-mono"
            :class="showTransformsPopover ? 'border-zinc-500 text-zinc-100' : ''"
            @click.stop="openTransformsPopover"
          >| Transforms</button>
        </div>
      </div>

      <!-- Preview table (full width) -->
      <div class="flex flex-col min-h-[300px] max-h-[50vh]">
        <div class="flex items-center justify-between mb-2 shrink-0">
          <span class="text-xs font-medium text-zinc-400 uppercase tracking-wider">Preview</span>
          <span v-if="loadingPreview" class="text-xs text-zinc-500 animate-pulse">Computing…</span>
        </div>

        <div class="flex-1 border border-zinc-700 rounded-lg overflow-hidden flex flex-col min-h-0">
          <table class="w-full">
            <thead class="sticky top-0 z-10">
              <tr class="border-b border-zinc-700 bg-zinc-800">
                <th class="text-left text-xs font-medium text-zinc-400 px-3 py-2 w-8">#</th>
                <th class="text-left text-xs font-medium text-zinc-400 px-3 py-2 w-[38%]">File Name</th>
                <th class="text-left text-xs font-medium text-zinc-400 px-3 py-2">New Name</th>
                <th class="text-right text-xs font-medium text-zinc-400 px-3 py-2 w-20">Size</th>
              </tr>
            </thead>
          </table>
          <div class="overflow-y-auto flex-1">
            <table class="w-full">
              <tbody>
                <tr
                  v-for="(item, index) in previewItems"
                  :key="item.original_name + index"
                  class="border-b border-zinc-800 last:border-0"
                  :class="index % 2 === 0 ? 'bg-zinc-900' : 'bg-zinc-800/20'"
                >
                  <td class="px-3 py-1.5 text-xs text-zinc-500 w-8 whitespace-nowrap">{{ index + 1 }}</td>
                  <td class="px-3 py-1.5 text-xs text-zinc-300 font-mono w-[38%] truncate max-w-0">
                    <span class="block truncate">{{ item.original_name }}</span>
                  </td>
                  <td class="px-3 py-1.5 text-xs font-mono truncate max-w-0">
                    <span v-if="item.error" class="text-red-400 block truncate" :title="item.error">{{ item.error }}</span>
                    <span v-else-if="item.new_name === item.original_name" class="text-zinc-600 italic">unchanged</span>
                    <span v-else class="text-emerald-400 block truncate" :title="item.new_name">{{ item.new_name }}</span>
                  </td>
                  <td class="px-3 py-1.5 text-xs text-right text-zinc-500 w-20 whitespace-nowrap">
                    {{ formatFileSize(files[index]?.size ?? 0) }}
                  </td>
                </tr>
                <tr v-if="previewItems.length === 0 && !loadingPreview">
                  <td colspan="4" class="px-3 py-8 text-center text-xs text-zinc-600">
                    Enter a template above to preview the renames
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <!-- Variable picker popover -->
    <Teleport to="body">
      <div
        v-if="showVarPicker"
        class="fixed z-[61] bg-zinc-900 border border-zinc-700 rounded-xl shadow-2xl w-96 max-w-[calc(100vw-1rem)]"
        :style="{ top: varPickerPos.top + 'px', left: `clamp(${varPickerPos.contentLeft}px, ${varPickerPos.left}px, calc(${varPickerPos.contentRight}px - 100%))` }"
        @click.stop
      >
          <!-- Category tabs -->
          <div class="flex border-b border-zinc-700">
            <button
              v-for="cat in VARIABLE_CATEGORIES"
              :key="cat.key"
              type="button"
              class="flex-1 py-2.5 text-xs font-medium transition-colors border-b-2 -mb-px"
              :class="activeCategory === cat.key ? 'border-current' : 'border-transparent text-zinc-500 hover:text-zinc-300'"
              :style="activeCategory === cat.key ? { color: cat.color, borderColor: cat.color } : {}"
              @click="activeCategory = cat.key"
            >{{ cat.label }}</button>
          </div>
          <!-- Variable grid -->
          <div class="p-2.5 grid grid-cols-2 gap-1.5">
            <button
              v-for="v in activeCategoryData.vars"
              :key="v.name"
              type="button"
              class="text-left rounded-lg border p-2.5 transition-all hover:brightness-125 active:scale-95"
              :style="{ background: activeCategoryData.color + '0a', borderColor: activeCategoryData.color + '30' }"
              @click="insertVariable(v.name)"
            >
              <div class="font-mono text-xs font-semibold" :style="{ color: activeCategoryData.color }">{{ '{' + v.name + '}' }}</div>
              <div class="text-xs text-zinc-500 leading-tight mt-0.5">{{ v.desc }}</div>
            </button>
          </div>
        </div>
    </Teleport>

    <!-- Transforms popover -->
    <Teleport to="body">
      <div
        v-if="showTransformsPopover"
        class="fixed z-[61] bg-zinc-900 border border-zinc-700 rounded-xl shadow-2xl p-4 w-[26rem] max-w-[calc(100vw-1rem)]"
        :style="{ top: transformsPos.top + 'px', left: `clamp(${transformsPos.contentLeft}px, ${transformsPos.left}px, calc(${transformsPos.contentRight}px - 100%))` }"
        @click.stop
      >
          <p class="text-xs font-semibold text-zinc-300 mb-3">Use after <code class="text-orange-400">|</code> inside a token — e.g. <code class="text-zinc-300">{name|slug}</code></p>
          <div class="grid grid-cols-1 gap-1">
            <div v-for="t in TRANSFORMS" :key="t.syntax" class="flex items-start gap-3">
              <code class="font-mono text-xs text-orange-400 w-28 shrink-0 pt-px">{{ t.syntax }}</code>
              <span class="text-xs text-zinc-400 leading-relaxed">{{ t.desc }}</span>
            </div>
          </div>
        </div>
    </Teleport>

    <!-- Presets popover -->
    <Teleport to="body">
      <div
        v-if="showPresetsPopover"
        class="fixed z-[61] bg-zinc-900 border border-zinc-700 rounded-xl shadow-2xl w-72 max-w-[calc(100vw-1rem)]"
        :style="{ top: presetsPos.top + 'px', right: `calc(100vw - ${presetsPos.contentRight}px)` }"
        @click.stop
      >
          <div class="p-3 flex flex-col gap-2">
            <span class="text-xs font-medium text-zinc-400 uppercase tracking-wider">Presets</span>

            <div v-if="presets.length === 0 && !showSavePreset" class="text-xs text-zinc-600 py-1">
              No presets saved yet
            </div>

            <div class="flex flex-wrap gap-1.5">
              <PresetPill
                v-for="preset in presets"
                :key="preset.id"
                :name="preset.name"
                :template="preset.template"
                :active="template === preset.template"
                @select="template = preset.template; showPresetsPopover = false"
                @delete="deletePreset(preset.id)"
              />
            </div>

            <div class="border-t border-zinc-700 pt-2">
              <template v-if="showSavePreset">
                <div class="flex flex-col gap-2">
                  <input
                    ref="savePresetInputRef"
                    v-model="savePresetName"
                    type="text"
                    placeholder="Preset name"
                    class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg px-3 py-2 text-sm border border-zinc-600 focus:outline-none focus:border-zinc-500 transition-colors"
                    @keydown.enter="confirmSavePreset"
                    @keydown.escape="showSavePreset = false"
                  />
                  <div class="flex gap-2">
                    <AppButton variant="primary" class="flex-1" @click="confirmSavePreset">Save</AppButton>
                    <AppButton @click="showSavePreset = false; savePresetName = ''">Cancel</AppButton>
                  </div>
                </div>
              </template>
              <AppButton v-else variant="ghost" @click="showSavePreset = true; savePresetName = ''">
                + Save current template
              </AppButton>
            </div>
          </div>
        </div>
    </Teleport>

    <template #footer>
      <div class="flex justify-end gap-2">
        <AppButton @click="emit('close')">Cancel</AppButton>
        <AppButton variant="primary" :disabled="!canApply || applying" @click="applyRenames()">
          {{ applying ? 'Applying…' : 'Apply' }}
        </AppButton>
      </div>
    </template>
  </AppModal>
</template>
