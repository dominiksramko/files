<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref } from 'vue'
import { VueDraggable } from 'vue-draggable-plus'
import type { FileEntry } from '@/types'
import { useEditableFile } from '@/composables/useFileContent'
import { dragOverNearestList } from '@/utils/sortable'
import {
  adoptBoardIds,
  DEFAULT_COLUMNS,
  makeColumn,
  parseKanban,
  serializeKanban,
  type KanbanBoard,
  type KanbanCard,
} from '@/utils/kanban'
import AppIcon from '@/components/ui/AppIcon.vue'
import EditableFileState from '@/components/files/EditableFileState.vue'
import KanbanCardModal from '@/components/files/KanbanCardModal.vue'
import KanbanColumn from '@/components/files/KanbanColumn.vue'

const props = defineProps<{ entry: FileEntry; active?: boolean }>()

const board = ref<KanbanBoard>({ preamble: '', columns: [] })

const cardCount = computed(() => board.value.columns.reduce((total, column) => total + column.cards.length, 0))

// Every label used on the board, offered as suggestions in the card editor.
const boardLabels = computed(() => {
  const unique = new Map<string, string>()
  for (const column of board.value.columns) {
    for (const card of column.cards) {
      for (const label of card.labels) unique.set(label.toLowerCase(), label)
    }
  }
  return [...unique.values()].sort((first, second) => first.localeCompare(second))
})

const { loading, loadError, tooLarge, saveLabel } = useEditableFile(() => props.entry, {
  state: board,
  parse: parseKanban,
  serialize: serializeKanban,
  onLoaded() {
    // A new / empty board starts with the usual three columns (saved on first edit).
    if (board.value.columns.length === 0) {
      board.value.columns = DEFAULT_COLUMNS.map((title) => makeColumn(title))
    }
  },
  carryOverIds: adoptBoardIds,
})

// ── Columns ──────────────────────────────────────────────────────────────────
const columnRefs = new Map<number, { focusTitle: () => void }>()
function setColumnRef(id: number, instance: unknown) {
  if (instance) columnRefs.set(id, instance as { focusTitle: () => void })
  else columnRefs.delete(id)
}

const boardScrollerRef = ref<HTMLElement | null>(null)

async function addColumn() {
  const column = makeColumn('')
  board.value.columns.push(column)
  await nextTick()
  boardScrollerRef.value?.scrollTo({ left: boardScrollerRef.value.scrollWidth, behavior: 'smooth' })
  columnRefs.get(column.id)?.focusTitle()
}

function removeColumn(columnId: number) {
  board.value.columns = board.value.columns.filter((column) => column.id !== columnId)
}

// ── Card drag: the whole height above and below a column counts as dropping
// into it, and the column the card would land in is outlined ─────────────────
const dropColumnId = ref<number | null>(null)

function followPointer(event: PointerEvent | TouchEvent) {
  const point = 'touches' in event ? event.touches[0] : event
  const scroller = boardScrollerRef.value
  if (!point || !scroller) return
  dragOverNearestList([...scroller.querySelectorAll<HTMLElement>('[data-kanban-cards]')], point.clientX, point.clientY)
}

function startCardDrag(columnId: number) {
  dropColumnId.value = columnId
  document.addEventListener('pointermove', followPointer)
  document.addEventListener('touchmove', followPointer, { passive: true })
}

function endCardDrag() {
  dropColumnId.value = null
  document.removeEventListener('pointermove', followPointer)
  document.removeEventListener('touchmove', followPointer)
}

onUnmounted(endCardDrag)

// ── Card editor ──────────────────────────────────────────────────────────────
const openedCardId = ref<number | null>(null)

// The opened card and the column it currently sits in (it may have been dragged
// or moved since it was opened); null once the card is gone.
const openedCard = computed(() => {
  for (const column of board.value.columns) {
    const card = column.cards.find((candidate) => candidate.id === openedCardId.value)
    if (card) return { card, column }
  }
  return null
})

const columnOptions = computed(() =>
  board.value.columns.map((column) => ({ label: column.title || 'Untitled column', value: column.id })),
)

function openCard(card: KanbanCard) {
  openedCardId.value = card.id
}

function deleteOpenedCard() {
  const opened = openedCard.value
  if (!opened) return
  opened.column.cards = opened.column.cards.filter((card) => card.id !== opened.card.id)
  openedCardId.value = null
}

function moveOpenedCard(targetColumnId: number) {
  const opened = openedCard.value
  const target = board.value.columns.find((column) => column.id === targetColumnId)
  if (!opened || !target) return
  opened.column.cards = opened.column.cards.filter((card) => card.id !== opened.card.id)
  target.cards.push(opened.card)
}
</script>

<template>
  <div class="flex w-full min-w-0 flex-col self-stretch" @click.stop>
    <!-- Toolbar -->
    <div
      class="flex shrink-0 items-center gap-2 border-b border-zinc-800/60 px-4 py-1.5 text-xs text-zinc-400"
      @click.stop
    >
      <AppIcon name="kanban" class="size-4 text-sky-400" />
      <span>{{ cardCount }} card{{ cardCount === 1 ? '' : 's' }}</span>
      <span class="ml-auto tabular-nums text-zinc-500">{{ saveLabel }}</span>
    </div>

    <!-- Board -->
    <div class="flex min-h-0 flex-1 flex-col bg-zinc-900">
      <EditableFileState :entry="entry" :loading="loading" :load-error="loadError" :too-large="tooLarge">
        <div
          ref="boardScrollerRef"
          data-no-carousel-swipe
          class="flex min-h-0 flex-1 snap-x snap-mandatory scroll-px-3 items-start gap-3 overflow-x-auto p-3 sm:snap-none"
        >
          <VueDraggable
            v-model="board.columns"
            class="flex h-full items-start gap-3"
            handle=".kanban-column-handle"
            direction="horizontal"
            :animation="180"
            :force-fallback="true"
            ghost-class="drag-ghost"
            fallback-class="drag-fallback"
          >
            <KanbanColumn
              v-for="column in board.columns"
              :key="column.id"
              :ref="(instance) => setColumnRef(column.id, instance)"
              v-model:title="column.title"
              v-model:cards="column.cards"
              :drop-target="dropColumnId === column.id"
              @remove="removeColumn(column.id)"
              @open-card="openCard"
              @card-drag-start="startCardDrag(column.id)"
              @card-drag-enter="dropColumnId = column.id"
              @card-drag-end="endCardDrag"
            />
          </VueDraggable>

          <button
            type="button"
            class="flex w-[85vw] shrink-0 snap-start items-center gap-2 rounded-xl border border-dashed border-zinc-700 px-3 py-2.5 text-sm text-zinc-400 transition-colors hover:border-zinc-500 hover:text-zinc-200 sm:w-72"
            @click="addColumn"
          >
            <AppIcon name="plus" class="size-4" />
            Add column
          </button>
        </div>
      </EditableFileState>
    </div>

    <KanbanCardModal
      v-if="openedCard"
      v-model:title="openedCard.card.title"
      v-model:description="openedCard.card.description"
      v-model:labels="openedCard.card.labels"
      v-model:due="openedCard.card.due"
      v-model:checklist="openedCard.card.checklist"
      :column-id="openedCard.column.id"
      :column-title="openedCard.column.title"
      :column-options="columnOptions"
      :board-labels="boardLabels"
      :entry-path="entry.path"
      @close="openedCardId = null"
      @delete="deleteOpenedCard"
      @move="moveOpenedCard"
    />
  </div>
</template>
