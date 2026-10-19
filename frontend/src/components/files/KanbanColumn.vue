<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { VueDraggable } from 'vue-draggable-plus'
import { makeCard, type KanbanCard as KanbanCardData } from '@/utils/kanban'
import AppButton from '@/components/ui/AppButton.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import KanbanCard from '@/components/files/KanbanCard.vue'

const title = defineModel<string>('title', { required: true })
const cards = defineModel<KanbanCardData[]>('cards', { required: true })

const emit = defineEmits<{
  remove: []
  openCard: [card: KanbanCardData]
}>()

const titleInputRef = ref<HTMLInputElement | null>(null)

function focusTitle() {
  titleInputRef.value?.focus()
}

// ── Delete (asks first when the column still has cards) ──────────────────────
const confirmingDelete = ref(false)

function requestDelete() {
  if (cards.value.length === 0) emit('remove')
  else confirmingDelete.value = true
}

// ── Add-card composer: stays open after Enter so cards can be added in a row ──
const composing     = ref(false)
const draftTitle    = ref('')
const composerRef   = ref<HTMLTextAreaElement | null>(null)
const cardListRef   = ref<HTMLElement | null>(null)

async function openComposer() {
  composing.value = true
  await nextTick()
  composerRef.value?.focus()
}

function closeComposer() {
  composing.value = false
  draftTitle.value = ''
}

async function submitDraft() {
  const cardTitle = draftTitle.value.trim()
  if (!cardTitle) {
    closeComposer()
    return
  }
  cards.value = [...cards.value, makeCard(cardTitle)]
  draftTitle.value = ''
  await nextTick()
  cardListRef.value?.scrollTo({ top: cardListRef.value.scrollHeight })
}

defineExpose({ focusTitle })
</script>

<template>
  <section class="group/column flex max-h-full w-[85vw] shrink-0 snap-start flex-col rounded-xl bg-zinc-800/50 sm:w-72">
    <!-- Header -->
    <div class="flex items-center gap-1 px-2 pb-1 pt-2">
      <button
        type="button"
        class="kanban-column-handle shrink-0 cursor-grab text-zinc-600 transition-opacity hover:text-zinc-300 active:cursor-grabbing sm:opacity-0 sm:group-hover/column:opacity-100"
        title="Drag to reorder"
      >
        <AppIcon name="gripVertical" class="size-4" />
      </button>
      <input
        ref="titleInputRef"
        v-model="title"
        type="text"
        placeholder="Column name"
        class="min-w-0 flex-1 rounded bg-transparent px-1.5 py-1 text-sm font-semibold text-zinc-100 placeholder-zinc-500 focus:bg-zinc-800 focus:outline-none"
        @keydown.enter="($event.target as HTMLInputElement).blur()"
      />
      <span class="shrink-0 px-1 text-xs tabular-nums text-zinc-500">{{ cards.length }}</span>
      <button
        v-if="!confirmingDelete"
        type="button"
        class="shrink-0 rounded p-1 text-zinc-600 transition-all hover:bg-zinc-700 hover:text-red-400 sm:opacity-0 sm:group-hover/column:opacity-100"
        title="Delete column"
        @click="requestDelete"
      >
        <AppIcon name="trash" class="size-4" />
      </button>
    </div>

    <!-- Inline delete confirmation -->
    <div
      v-if="confirmingDelete"
      class="mx-2 mb-2 flex items-center gap-2 rounded-lg bg-red-950/40 px-2.5 py-2 text-xs text-red-200"
    >
      <span class="flex-1">Delete column and {{ cards.length }} card{{ cards.length === 1 ? '' : 's' }}?</span>
      <button type="button" class="font-medium text-red-300 hover:text-red-200" @click="emit('remove')">Delete</button>
      <button type="button" class="text-zinc-400 hover:text-zinc-200" @click="confirmingDelete = false">Cancel</button>
    </div>

    <!-- Cards -->
    <div ref="cardListRef" class="min-h-0 flex-1 overflow-y-auto px-2">
      <VueDraggable
        v-model="cards"
        group="kanban-cards"
        class="flex min-h-10 flex-col gap-2 pb-1"
        :animation="180"
        :force-fallback="true"
        :delay="150"
        :delay-on-touch-only="true"
        ghost-class="drag-ghost"
        fallback-class="drag-fallback"
      >
        <KanbanCard v-for="card in cards" :key="card.id" :card="card" @open="emit('openCard', card)" />
      </VueDraggable>
    </div>

    <!-- Add card -->
    <div class="p-2 pt-1">
      <div v-if="composing">
        <textarea
          ref="composerRef"
          v-model="draftTitle"
          rows="2"
          placeholder="Card title"
          class="w-full resize-none rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 placeholder-zinc-500 focus:border-zinc-500 focus:outline-none"
          @keydown.enter.prevent="submitDraft"
          @keydown.escape.stop="closeComposer"
          @blur="draftTitle.trim() ? null : closeComposer()"
        />
        <div class="mt-1.5 flex items-center gap-2">
          <AppButton variant="primary" @mousedown.prevent @click="submitDraft">Add card</AppButton>
          <button
            type="button"
            class="rounded p-1 text-zinc-400 transition-colors hover:text-white"
            title="Cancel"
            @mousedown.prevent
            @click="closeComposer"
          >
            <AppIcon name="x" class="size-4" />
          </button>
        </div>
      </div>
      <button
        v-else
        type="button"
        class="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-sm text-zinc-400 transition-colors hover:bg-zinc-700/50 hover:text-zinc-200"
        @click="openComposer"
      >
        <AppIcon name="plus" class="size-4" />
        Add card
      </button>
    </div>
  </section>
</template>
