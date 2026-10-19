<script setup lang="ts">
import { computed, ref } from 'vue'
import { VueDraggable } from 'vue-draggable-plus'
import type { FileEntry } from '@/types'
import { useChecklistEditor } from '@/composables/useChecklistEditor'
import { useEditableFile } from '@/composables/useFileContent'
import AppIcon from '@/components/ui/AppIcon.vue'
import EditableFileState from '@/components/files/EditableFileState.vue'
import TodoRow from '@/components/files/TodoRow.vue'

const props = defineProps<{ entry: FileEntry; active?: boolean }>()

interface TodoItem {
  id: number
  text: string
  done: boolean
}

let nextId = 1
function makeItem(text: string, done: boolean): TodoItem {
  return { id: nextId++, text, done }
}

const items         = ref<TodoItem[]>([])
const completedOpen = ref(true)

const completedItems = computed(() => items.value.filter((item) => item.done))

// Writable list of the open items, in display order. VueDraggable mutates this on
// reorder; the setter rebuilds `items` keeping completed tasks pinned at the bottom.
const activeItems = computed<TodoItem[]>({
  get: () => items.value.filter((item) => !item.done),
  set: (reordered) => { items.value = [...reordered, ...completedItems.value] },
})

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

// ── Item operations ──────────────────────────────────────────────────────────
const { setRowRef, focusItem, toggle, removeItem, newItemAfter, addItem, onBackspaceEmpty } =
  useChecklistEditor(items, () => makeItem('', false))

function clearCompleted() {
  items.value = items.value.filter((item) => !item.done)
}

// ── Load / auto-save ─────────────────────────────────────────────────────────
const { loading, loadError, tooLarge, saveLabel } = useEditableFile(() => props.entry, {
  state: items,
  parse,
  serialize,
  onLoaded() {
    // Start a fresh / empty file with one row, like Google Keep. Only steal focus
    // when this is the active panel — neighbouring carousel panels mount off-screen
    // and focusing them would scroll/break the preview slider.
    if (items.value.length === 0) {
      const fresh = makeItem('', false)
      items.value.push(fresh)
      if (props.active !== false) focusItem(fresh.id)
    }
  },
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
      <EditableFileState :entry="entry" :loading="loading" :load-error="loadError" :too-large="tooLarge">
        <div class="mx-auto w-full max-w-2xl">
          <!-- Active items (sortable) -->
          <VueDraggable
            v-model="activeItems"
            tag="ul"
            handle=".todo-drag-handle"
            :animation="180"
            :force-fallback="true"
            ghost-class="drag-ghost"
            fallback-class="drag-fallback"
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
              @backspace-empty="onBackspaceEmpty(item, activeItems)"
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
                @backspace-empty="onBackspaceEmpty(item, completedItems)"
              />
            </ul>
          </div>
        </div>
      </EditableFileState>
    </div>
  </div>
</template>
