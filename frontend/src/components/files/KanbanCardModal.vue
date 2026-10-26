<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { VueDraggable } from 'vue-draggable-plus'
import type { FileEntry } from '@/types'
import { useChecklistEditor } from '@/composables/useChecklistEditor'
import { useToast } from '@/composables/useToast'
import { droppedFiles, leftOutFoldersMessage } from '@/utils/drop'
import { catBg, fileCategory, formatSize } from '@/utils/formatters'
import {
  attachmentFolder,
  attachmentImageUrl,
  makeChecklistItem,
  labelColor,
  normalizeLabel,
  resolveAttachment,
  type KanbanChecklistItem,
} from '@/utils/kanban'
import { renderMarkdown } from '@/utils/markdown'
import { nameOf, parentOf } from '@/utils/paths'
import AppButton from '@/components/ui/AppButton.vue'
import AppCheckbox from '@/components/ui/AppCheckbox.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppModal from '@/components/ui/AppModal.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import FileIcon from '@/components/files/FileIcon.vue'
import FilePickerModal from '@/components/files/FilePickerModal.vue'
import TodoRow from '@/components/files/TodoRow.vue'

// Edits go straight back to the board through the models, so the viewer's
// auto-save picks them up — there is no Save button.
const title       = defineModel<string>('title', { required: true })
const description = defineModel<string>('description', { required: true })
const labels      = defineModel<string[]>('labels', { required: true })
const due         = defineModel<string | null>('due', { required: true })
const dueDone     = defineModel<boolean>('dueDone', { required: true })
const attachments = defineModel<string[]>('attachments', { required: true })
const checklist   = defineModel<KanbanChecklistItem[]>('checklist', { required: true })

const props = defineProps<{
  columnId: number
  columnTitle: string
  columnOptions: { label: string; value: number }[]
  boardLabels: string[] // every label used on the board, offered as suggestions
  entryPath: string // the .kanban file: links in the description and attachments are relative to it
  // An attached file's details: undefined while unknown, null when it's missing.
  attachmentEntry: (path: string) => FileEntry | null | undefined
  uploadPercent: number | null // how far an upload to this card has got; null when none is running
}>()

const emit = defineEmits<{
  close: []
  delete: []
  move: [columnId: number]
  upload: [files: File[]]
  attach: [paths: string[]] // files chosen from the user's folder
  openAttachment: [path: string]
}>()

const { show: showToast } = useToast()

// ── Due date: clearing it clears "done" too, so no stray @done stays in the file ──
function setDue(value: string | null) {
  due.value = value
  if (!value) dueDone.value = false
}

// ── Labels ───────────────────────────────────────────────────────────────────
const labelDraft = ref('')
const labelSuggestions = computed(() =>
  props.boardLabels.filter((label) => !labels.value.some((own) => own.toLowerCase() === label.toLowerCase())),
)

function addLabel() {
  if (!labelDraft.value.trim()) return
  const label = normalizeLabel(labelDraft.value)
  if (!label) {
    showToast('Labels must start with a letter', 'error')
    return
  }
  if (!labels.value.some((own) => own.toLowerCase() === label.toLowerCase())) {
    labels.value = [...labels.value, label]
  }
  labelDraft.value = ''
}

function removeLabel(label: string) {
  labels.value = labels.value.filter((own) => own !== label)
}

// ── Description ──────────────────────────────────────────────────────────────
const editingDescription = ref(false)
const descriptionRef = ref<HTMLTextAreaElement | null>(null)
const renderedDescription = computed(() => renderMarkdown(description.value, props.entryPath, { lineBreaks: true }))

async function startEditingDescription() {
  editingDescription.value = true
  await nextTick()
  descriptionRef.value?.focus()
}

// ── Attachments: removing one leaves the file where it is ────────────────────
const attachmentRows = computed(() =>
  attachments.value.map((link) => {
    const path = resolveAttachment(props.entryPath, link)
    const entry = path === null ? null : props.attachmentEntry(path)
    const category = entry ? fileCategory(entry.mime, entry.name) : 'other'
    return {
      link,
      path,
      entry,
      name: nameOf(link) || link,
      imageUrl: entry && category === 'image' ? attachmentImageUrl(entry) : null,
      background: catBg[category],
    }
  }),
)

// A hand-edited file may list a file twice, so rows go by position.
function removeAttachment(index: number) {
  attachments.value = attachments.value.filter((_, position) => position !== index)
}

const uploadFolder = computed(() => attachmentFolder(props.entryPath))

const uploadInput = ref<HTMLInputElement | null>(null)

function onUploadInput() {
  const files = Array.from(uploadInput.value?.files ?? [])
  if (files.length) emit('upload', files)
  if (uploadInput.value) uploadInput.value.value = ''
}

const choosingFiles = ref(false)

function onChosen(paths: string[]) {
  choosingFiles.value = false
  emit('attach', paths)
}

// Files dropped anywhere on the card are uploaded to it.
const fileDragDepth = ref(0)
const isFileDrag = (event: DragEvent) => event.dataTransfer?.types.includes('Files') === true

function onFileDragEnter(event: DragEvent) {
  if (isFileDrag(event)) fileDragDepth.value++
}

function onFileDragOver(event: DragEvent) {
  if (!isFileDrag(event)) return
  event.preventDefault()
  event.dataTransfer!.dropEffect = 'copy'
}

function onFileDrop(event: DragEvent) {
  fileDragDepth.value = 0
  if (!isFileDrag(event)) return
  event.preventDefault()
  const { files, folders } = droppedFiles(event.dataTransfer!)
  const leftOut = leftOutFoldersMessage(folders)
  if (leftOut) showToast(leftOut, 'info')
  if (files.length) emit('upload', files)
}

// ── Checklist ────────────────────────────────────────────────────────────────
const { setRowRef, toggle, removeItem, newItemAfter, addItem, onBackspaceEmpty } =
  useChecklistEditor(checklist, () => makeChecklistItem(''))

const checklistDone = computed(() => checklist.value.filter((item) => item.done).length)
const checklistPercent = computed(() =>
  checklist.value.length ? Math.round((checklistDone.value / checklist.value.length) * 100) : 0,
)

// ── Footer ───────────────────────────────────────────────────────────────────
const confirmingDelete = ref(false)

function onMoveSelect(columnId: string | number) {
  if (columnId !== props.columnId) emit('move', Number(columnId))
}
</script>

<template>
  <AppModal :title="columnTitle || 'Card'" size="lg" @close="emit('close')">
    <div
      class="flex flex-col gap-5"
      @dragenter="onFileDragEnter"
      @dragleave="fileDragDepth = Math.max(0, fileDragDepth - 1)"
      @dragover="onFileDragOver"
      @drop="onFileDrop"
    >
      <!-- Title -->
      <textarea
        v-model="title"
        rows="1"
        placeholder="Card title"
        class="field-sizing-content w-full resize-none rounded-lg border border-transparent bg-transparent px-2 py-1 text-lg font-semibold text-zinc-100 placeholder-zinc-500 transition-colors focus:border-zinc-600 focus:bg-zinc-800 focus:outline-none"
        @keydown.enter.prevent="($event.target as HTMLTextAreaElement).blur()"
      />

      <div class="grid gap-4 sm:grid-cols-[1fr_auto]">
        <!-- Labels -->
        <div>
          <h4 class="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-zinc-400">
            <AppIcon name="tag" class="size-3.5" /> Labels
          </h4>
          <div class="flex flex-wrap items-center gap-1.5">
            <span
              v-for="label in labels"
              :key="label"
              class="flex items-center gap-1 rounded py-0.5 pl-2 pr-1 text-xs font-medium"
              :class="labelColor(label)"
            >
              {{ label }}
              <button type="button" class="rounded opacity-70 hover:opacity-100" :title="`Remove ${label}`" @click="removeLabel(label)">
                <AppIcon name="x" class="size-3" />
              </button>
            </span>
            <input
              v-model="labelDraft"
              type="text"
              list="kanban-label-suggestions"
              placeholder="Add label"
              class="w-28 rounded-lg border border-transparent bg-zinc-800 px-2.5 py-1 text-xs text-zinc-100 placeholder-zinc-500 transition-colors focus:border-zinc-600 focus:outline-none"
              @keydown.enter.prevent="addLabel"
              @change="addLabel"
            />
            <datalist id="kanban-label-suggestions">
              <option v-for="label in labelSuggestions" :key="label" :value="label" />
            </datalist>
          </div>
        </div>

        <!-- Due date -->
        <div>
          <h4 class="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-zinc-400">
            <AppIcon name="calendar" class="size-3.5" /> Due date
          </h4>
          <div class="flex items-center gap-1">
            <input
              type="date"
              :value="due ?? ''"
              class="rounded-lg border border-transparent bg-zinc-800 px-2.5 py-1 text-xs text-zinc-100 [color-scheme:dark] focus:border-zinc-600 focus:outline-none"
              @change="setDue(($event.target as HTMLInputElement).value || null)"
            />
            <button
              v-if="due"
              type="button"
              class="rounded p-1 text-zinc-500 transition-colors hover:text-zinc-200"
              title="Clear due date"
              @click="setDue(null)"
            >
              <AppIcon name="x" class="size-3.5" />
            </button>
            <label v-if="due" class="ml-2 flex cursor-pointer items-center gap-1.5 text-xs text-zinc-300">
              <AppCheckbox v-model="dueDone" />
              Done
            </label>
          </div>
        </div>
      </div>

      <!-- Description -->
      <div>
        <div class="mb-1.5 flex items-center justify-between">
          <h4 class="flex items-center gap-1.5 text-xs font-medium text-zinc-400">
            <AppIcon name="alignLeft" class="size-3.5" /> Description
          </h4>
          <button
            v-if="description.trim()"
            type="button"
            class="rounded px-2 py-0.5 text-xs text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
            @click="editingDescription ? (editingDescription = false) : startEditingDescription()"
          >{{ editingDescription ? 'Preview' : 'Edit' }}</button>
        </div>
        <textarea
          v-if="editingDescription"
          ref="descriptionRef"
          v-model="description"
          rows="6"
          placeholder="Add a more detailed description… (Markdown supported)"
          class="field-sizing-content min-h-32 w-full resize-y rounded-lg border border-transparent bg-zinc-800 px-3 py-2 font-mono text-sm leading-relaxed text-zinc-100 placeholder-zinc-500 transition-colors focus:border-zinc-600 focus:outline-none"
        />
        <!-- eslint-disable-next-line vue/no-v-html -->
        <article
          v-else-if="description.trim()"
          class="markdown-body cursor-text rounded-lg px-1 text-sm"
          @dblclick="startEditingDescription"
          v-html="renderedDescription"
        />
        <button
          v-else
          type="button"
          class="w-full rounded-lg bg-zinc-800/60 px-3 py-3 text-left text-sm text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-zinc-300"
          @click="startEditingDescription"
        >Add a more detailed description…</button>
      </div>

      <!-- Attachments -->
      <div
        class="-m-2 rounded-lg p-2 transition-shadow"
        :class="{ 'ring-2 ring-blue-500/60': fileDragDepth > 0 }"
        data-attachments
      >
        <div class="mb-1.5 flex items-center justify-between gap-2">
          <h4 class="flex items-center gap-1.5 text-xs font-medium text-zinc-400">
            <AppIcon name="paperclip" class="size-3.5" /> Attachments
          </h4>
          <div class="flex items-center gap-1">
            <span v-if="uploadPercent !== null" class="mr-1 flex items-center gap-1.5 text-xs tabular-nums text-zinc-400">
              <AppSpinner size="sm" /> Uploading… {{ uploadPercent }}%
            </span>
            <button
              type="button"
              class="flex items-center gap-1 rounded px-2 py-0.5 text-xs text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
              :title="`Upload into “${nameOf(uploadFolder)}”, next to the board`"
              @click="uploadInput?.click()"
            >
              <AppIcon name="upload" class="size-3.5" /> Upload
            </button>
            <button
              type="button"
              class="flex items-center gap-1 rounded px-2 py-0.5 text-xs text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
              @click="choosingFiles = true"
            >
              <AppIcon name="folder" class="size-3.5" /> Choose from my files
            </button>
            <input ref="uploadInput" type="file" multiple class="hidden" @change="onUploadInput" />
          </div>
        </div>
        <ul v-if="attachmentRows.length" class="space-y-1">
          <li
            v-for="(row, index) in attachmentRows"
            :key="`${index}:${row.link}`"
            class="group/attachment flex items-center gap-3 rounded-lg px-1 py-1 transition-colors"
            :class="row.entry ? 'cursor-pointer hover:bg-zinc-800/60' : ''"
            :title="row.entry ? `Open ${row.name}` : row.link"
            @click="row.entry && row.path !== null && emit('openAttachment', row.path)"
          >
            <div class="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded" :class="row.background">
              <img v-if="row.imageUrl" :src="row.imageUrl" alt="" loading="lazy" class="size-full object-cover" />
              <FileIcon v-else :mime="row.entry?.mime ?? ''" :name="row.name" :is-dir="row.entry?.is_dir ?? false" />
            </div>
            <div class="min-w-0 flex-1">
              <p class="truncate text-sm" :class="row.entry === null ? 'text-zinc-500 line-through' : 'text-zinc-200'">{{ row.name }}</p>
              <p class="truncate text-xs text-zinc-500">
                <template v-if="row.entry === null">Not found at {{ row.link }}</template>
                <template v-else-if="row.entry">{{ row.entry.is_dir ? 'Folder' : formatSize(row.entry.size) }} · {{ parentOf(row.link) || 'next to the board' }}</template>
                <template v-else>&nbsp;</template>
              </p>
            </div>
            <button
              type="button"
              class="shrink-0 rounded p-1 text-zinc-500 transition-all hover:bg-zinc-700 hover:text-zinc-200 focus-visible:opacity-100 sm:opacity-0 sm:group-hover/attachment:opacity-100"
              :title="`Remove ${row.name} from the card (the file stays)`"
              :aria-label="`Remove ${row.name}`"
              @click.stop="removeAttachment(index)"
            >
              <AppIcon name="x" class="size-4" />
            </button>
          </li>
        </ul>
        <p v-else class="rounded-lg bg-zinc-800/60 px-3 py-3 text-sm text-zinc-500">No files attached</p>
      </div>

      <!-- Checklist -->
      <div>
        <div class="mb-1.5 flex items-center justify-between">
          <h4 class="flex items-center gap-1.5 text-xs font-medium text-zinc-400">
            <AppIcon name="listChecks" class="size-3.5" /> Checklist
          </h4>
          <span v-if="checklist.length" class="text-xs tabular-nums text-zinc-500">
            {{ checklistDone }}/{{ checklist.length }}
          </span>
        </div>
        <div v-if="checklist.length" class="mb-2 h-1 overflow-hidden rounded-full bg-zinc-800">
          <div
            class="h-full rounded-full transition-all"
            :class="checklistPercent === 100 ? 'bg-emerald-500' : 'bg-blue-500'"
            :style="{ width: `${checklistPercent}%` }"
          />
        </div>
        <VueDraggable
          v-model="checklist"
          tag="ul"
          handle=".todo-drag-handle"
          :animation="180"
          :force-fallback="true"
          ghost-class="drag-ghost"
          fallback-class="drag-fallback"
        >
          <TodoRow
            v-for="item in checklist"
            :key="item.id"
            :ref="(instance) => setRowRef(item.id, instance)"
            v-model:text="item.text"
            :done="item.done"
            :draggable="true"
            @toggle="toggle(item)"
            @remove="removeItem(item.id)"
            @enter="newItemAfter(item)"
            @backspace-empty="onBackspaceEmpty(item)"
          />
        </VueDraggable>
        <button
          type="button"
          class="mt-1 flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-sm text-zinc-400 transition-colors hover:bg-zinc-800/40 hover:text-zinc-200"
          @click="addItem"
        >
          <AppIcon name="plus" class="size-4" />
          Add item
        </button>
      </div>
    </div>

    <template #footer>
      <div class="flex flex-wrap items-center gap-3">
        <div class="flex items-center gap-2 text-xs text-zinc-400">
          Move to
          <div class="w-40">
            <AppSelect :model-value="columnId" :options="columnOptions" @update:model-value="onMoveSelect" />
          </div>
        </div>
        <div class="ml-auto flex items-center gap-2">
          <template v-if="confirmingDelete">
            <span class="text-xs text-zinc-400">Delete this card?</span>
            <AppButton @click="confirmingDelete = false">Cancel</AppButton>
            <AppButton variant="destructive" @click="emit('delete')">Delete</AppButton>
          </template>
          <AppButton v-else variant="destructive" @click="confirmingDelete = true">Delete card</AppButton>
        </div>
      </div>
    </template>

    <!-- Inside the card's modal, so it's the one on top -->
    <FilePickerModal
      v-if="choosingFiles"
      title="Attach files"
      choose-label="Attach"
      :folder="parentOf(entryPath)"
      @close="choosingFiles = false"
      @choose="onChosen"
    />
  </AppModal>
</template>
