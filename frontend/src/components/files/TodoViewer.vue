<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { VueDraggable } from 'vue-draggable-plus'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { useFilesStore } from '@/stores/files'
import { useToast } from '@/composables/useToast'
import { debounce } from '@/utils/debounce'
import { formatSize } from '@/utils/formatters'
import AppIcon from '@/components/ui/AppIcon.vue'
import TodoRow from '@/components/files/TodoRow.vue'

const props = defineProps<{ entry: FileEntry; active?: boolean }>()

const filesStore = useFilesStore()
const { show: showToast } = useToast()

const TEXT_SIZE_LIMIT = 2 * 1024 * 1024 // 2 MB

interface TodoItem {
  id: number
  text: string
  done: boolean
}

let nextId = 1
function makeItem(text: string, done: boolean): TodoItem {
  return { id: nextId++, text, done }
}

const items        = ref<TodoItem[]>([])
const loading      = ref(false)
const loadError    = ref(false)
const tooLarge     = ref(false)
const ready        = ref(false) // gates auto-save until the initial content is loaded
const completedOpen = ref(true)
const saveState    = ref<'idle' | 'saving' | 'saved'>('idle')

const completedItems = computed(() => items.value.filter((item) => item.done))

// Writable list of the open items, in display order. VueDraggable mutates this on
// reorder; the setter rebuilds `items` keeping completed tasks pinned at the bottom.
const activeItems = computed<TodoItem[]>({
  get: () => items.value.filter((item) => !item.done),
  set: (reordered) => { items.value = [...reordered, ...completedItems.value] },
})

const saveLabel = computed(() =>
  saveState.value === 'saving' ? 'Saving…' : saveState.value === 'saved' ? 'Saved' : '',
)

// ── Parse / serialize ───────────────────────────────────────────────────────
// On-disk format: one task per line, "[ ] text" (open) or "[x] text" (done).
const ITEM_RE = /^\s*\[([ xX])\]\s?(.*)$/

function parse(content: string): TodoItem[] {
  const parsed: TodoItem[] = []
  for (const line of content.split('\n')) {
    if (line.trim() === '') continue
    const match = ITEM_RE.exec(line)
    if (match) {
      parsed.push(makeItem(match[2] ?? '', match[1]!.toLowerCase() === 'x'))
    } else {
      // Non-checkbox line — keep its content so hand-edited files lose nothing.
      parsed.push(makeItem(line.trim(), false))
    }
  }
  return parsed
}

function serialize(list: TodoItem[]): string {
  // Active items first (in display order), completed items at the bottom.
  const ordered = [...list.filter((item) => !item.done), ...list.filter((item) => item.done)]
  if (ordered.length === 0) return ''
  return ordered.map((item) => `${item.done ? '[x]' : '[ ]'} ${item.text}`.trimEnd()).join('\n') + '\n'
}

// ── Row focus management ─────────────────────────────────────────────────────
const rowRefs = new Map<number, { focus: () => void }>()
function setRowRef(id: number, instance: unknown) {
  if (instance) rowRefs.set(id, instance as { focus: () => void })
  else rowRefs.delete(id)
}
async function focusItem(id: number) {
  await nextTick()
  rowRefs.get(id)?.focus()
}

// ── Item operations ──────────────────────────────────────────────────────────
function toggle(item: TodoItem) {
  item.done = !item.done
}

function removeItem(id: number) {
  const index = items.value.findIndex((item) => item.id === id)
  if (index !== -1) items.value.splice(index, 1)
}

function newItemAfter(item: TodoItem) {
  const index = items.value.findIndex((entry) => entry.id === item.id)
  const fresh = makeItem('', false)
  items.value.splice(index + 1, 0, fresh)
  focusItem(fresh.id)
}

function addItem() {
  const fresh = makeItem('', false)
  items.value.push(fresh)
  focusItem(fresh.id)
}

function onBackspaceEmpty(item: TodoItem) {
  const actives = activeItems.value
  const activeIndex = actives.findIndex((entry) => entry.id === item.id)
  const previous = activeIndex > 0 ? actives[activeIndex - 1] : null
  removeItem(item.id)
  if (previous) focusItem(previous.id)
}

function clearCompleted() {
  items.value = items.value.filter((item) => !item.done)
}

// ── Auto-save ────────────────────────────────────────────────────────────────
let pendingSave = false

async function doSave() {
  pendingSave = false
  saveState.value = 'saving'
  try {
    await filesApi.writeContent(props.entry.path, serialize(items.value))
    saveState.value = 'saved'
    filesStore.refresh()
  } catch (err) {
    saveState.value = 'idle'
    showToast((err as Error).message, 'error')
  }
}
const debouncedSave = debounce(doSave, 800)

watch(
  items,
  () => {
    if (!ready.value) return
    pendingSave = true
    saveState.value = 'saving'
    debouncedSave()
  },
  { deep: true },
)

// ── Load ─────────────────────────────────────────────────────────────────────
let fetchController: AbortController | null = null

onMounted(async () => {
  if (props.entry.size > TEXT_SIZE_LIMIT) {
    tooLarge.value = true
    return
  }
  loading.value = true
  fetchController = new AbortController()
  try {
    const response = await fetch(filesApi.streamUrl(props.entry.path, props.entry.mod_time), {
      signal: fetchController.signal,
    })
    if (!response.ok) throw new Error()
    items.value = parse(await response.text())
    // Start a fresh / empty file with one row, like Google Keep. Only steal focus
    // when this is the active panel — neighbouring carousel panels mount off-screen
    // and focusing them would scroll/break the preview slider.
    if (items.value.length === 0) {
      const fresh = makeItem('', false)
      items.value.push(fresh)
      if (props.active !== false) focusItem(fresh.id)
    }
  } catch (err) {
    if ((err as Error).name !== 'AbortError') loadError.value = true
  } finally {
    loading.value = false
    await nextTick()
    ready.value = true
  }
})

onBeforeUnmount(() => {
  fetchController?.abort()
  debouncedSave.cancel()
  if (pendingSave) doSave()
})
</script>

<template>
  <div class="flex w-full min-w-0 flex-col self-stretch" @click.stop>
    <!-- Toolbar -->
    <div
      class="flex shrink-0 items-center gap-2 border-b border-zinc-800/60 px-4 py-1.5 text-xs text-zinc-400"
      @click.stop
    >
      <AppIcon name="listChecks" class="size-4 text-emerald-400" />
      <span>{{ activeItems.length }} open</span>
      <span class="ml-auto tabular-nums text-zinc-500">{{ saveLabel }}</span>
    </div>

    <!-- Content -->
    <div class="min-h-0 flex-1 overflow-y-auto bg-zinc-900 px-3 py-4">
      <div v-if="loading" class="flex h-32 items-center justify-center text-sm text-zinc-400">
        Loading…
      </div>
      <div v-else-if="loadError" class="flex h-32 items-center justify-center text-sm text-red-400">
        Failed to load file content.
      </div>
      <div
        v-else-if="tooLarge"
        class="flex h-32 flex-col items-center justify-center gap-3 text-sm text-zinc-400"
      >
        <p>File is too large to edit ({{ formatSize(entry.size) }})</p>
        <a
          :href="filesApi.downloadUrl(entry.path, entry.mod_time)"
          target="_blank"
          class="text-blue-400 underline hover:text-blue-300"
          @click.stop
        >Download to view</a>
      </div>

      <div v-else class="mx-auto w-full max-w-2xl">
        <!-- Active items (sortable) -->
        <VueDraggable
          v-model="activeItems"
          tag="ul"
          handle=".todo-drag-handle"
          :animation="180"
          :force-fallback="true"
          ghost-class="todo-ghost"
          fallback-class="todo-fallback"
        >
          <TodoRow
            v-for="item in activeItems"
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

        <!-- Completed group -->
        <div v-if="completedItems.length" class="mt-4 border-t border-zinc-800 pt-3">
          <div class="flex items-center justify-between gap-2">
            <button
              type="button"
              class="flex items-center gap-1.5 px-1.5 text-xs font-medium text-zinc-400 transition-colors hover:text-zinc-200"
              @click="completedOpen = !completedOpen"
            >
              <AppIcon
                name="chevronDown"
                class="size-3.5 transition-transform"
                :class="completedOpen ? '' : '-rotate-90'"
              />
              Completed ({{ completedItems.length }})
            </button>
            <button
              type="button"
              class="flex items-center gap-1 rounded px-1.5 py-1 text-xs text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-red-400"
              title="Delete all checked items"
              @click="clearCompleted"
            >
              <AppIcon name="trash" class="size-3.5" />
              Delete completed
            </button>
          </div>
          <ul v-if="completedOpen" class="mt-1">
            <TodoRow
              v-for="item in completedItems"
              :key="item.id"
              :ref="(instance) => setRowRef(item.id, instance)"
              v-model:text="item.text"
              :done="item.done"
              @toggle="toggle(item)"
              @remove="removeItem(item.id)"
              @enter="newItemAfter(item)"
              @backspace-empty="onBackspaceEmpty(item)"
            />
          </ul>
        </div>
      </div>
    </div>
  </div>
</template>

<!-- Global (not scoped): SortableJS clones the dragged row onto document.body,
     so the fallback/ghost styling must be reachable outside this component's scope. -->
<style>
/* The faded placeholder left in the list while dragging. */
.todo-ghost { opacity: 0.35; }
/* The lifted clone that follows the pointer. */
.todo-fallback {
  border: 1px solid #3f3f46;
  border-radius: 0.5rem;
  box-shadow: 0 12px 28px rgba(0, 0, 0, 0.55);
  cursor: grabbing;
}
</style>
