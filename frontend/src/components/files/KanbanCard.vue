<script setup lang="ts">
import { computed } from 'vue'
import { dueStatus, formatDue, labelColor, type KanbanCard } from '@/utils/kanban'
import AppIcon from '@/components/ui/AppIcon.vue'

// coverUrl: an attached image shown across the top of the card.
const props = defineProps<{ card: KanbanCard; coverUrl?: string | null }>()

const emit = defineEmits<{
  open: []
  toggleDueDone: []
  // Open the card menu at this point on screen (right-click, or the ⋯ button).
  menu: [x: number, y: number]
}>()

const checklistDone = computed(() => props.card.checklist.filter((item) => item.done).length)
const hasDescription = computed(() => props.card.description.trim() !== '')
const hasBadges = computed(() =>
  props.card.due || hasDescription.value || props.card.attachments.length || props.card.checklist.length,
)

const dueClass = computed(() => {
  if (!props.card.due) return ''
  return {
    done:     'bg-emerald-500/20 text-emerald-300',
    overdue:  'bg-red-500/20 text-red-300',
    today:    'bg-amber-500/20 text-amber-300',
    upcoming: 'text-zinc-400',
  }[dueStatus(props.card.due, props.card.dueDone)]
})

function openMenuBelow(event: MouseEvent) {
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  emit('menu', rect.left, rect.bottom + 4)
}
</script>

<template>
  <div
    role="button"
    tabindex="0"
    class="group/card relative cursor-pointer select-none rounded-lg border border-zinc-700/60 bg-zinc-800 py-2 pl-3 pr-7 transition-colors hover:border-zinc-500 focus:border-zinc-500 focus:outline-none"
    @click="emit('open')"
    @keydown.enter="emit('open')"
    @contextmenu.prevent="emit('menu', $event.clientX, $event.clientY)"
  >
    <button
      type="button"
      class="absolute right-1 top-1 z-10 rounded p-1 transition-all hover:bg-zinc-700 hover:text-zinc-200 focus-visible:opacity-100 sm:opacity-0 sm:group-hover/card:opacity-100"
      :class="coverUrl ? 'bg-zinc-900/70 text-zinc-200' : 'text-zinc-500'"
      title="Card actions"
      aria-label="Card actions"
      @click.stop="openMenuBelow"
      @keydown.enter.stop
    >
      <AppIcon name="ellipsis" class="size-4" />
    </button>

    <img
      v-if="coverUrl"
      :src="coverUrl"
      alt=""
      loading="lazy"
      draggable="false"
      class="-ml-3 -mr-7 -mt-2 mb-2 block h-32 w-[calc(100%+2.5rem)] max-w-none rounded-t-lg object-cover"
      data-card-cover
    />

    <div v-if="card.labels.length" class="mb-1.5 flex flex-wrap gap-1">
      <span
        v-for="label in card.labels"
        :key="label"
        class="rounded px-1.5 py-0.5 text-[11px] font-medium leading-none"
        :class="labelColor(label)"
      >{{ label }}</span>
    </div>

    <p class="break-words text-sm leading-snug" :class="card.title ? 'text-zinc-100' : 'italic text-zinc-500'">
      {{ card.title || 'Untitled' }}
    </p>

    <div v-if="hasBadges" class="mt-1.5 flex flex-wrap items-center gap-x-2.5 gap-y-1 text-xs text-zinc-400">
      <button
        v-if="card.due"
        type="button"
        class="flex items-center gap-1 rounded px-1 py-0.5 transition-colors hover:bg-zinc-700"
        :class="dueClass"
        :title="card.dueDone ? 'Done — click to mark not done' : 'Click to mark done'"
        @click.stop="emit('toggleDueDone')"
        @keydown.enter.stop
      >
        <AppIcon :name="card.dueDone ? 'checkCircle' : 'calendar'" class="size-3.5" />
        {{ formatDue(card.due) }}
      </button>
      <span v-if="hasDescription" title="Has a description">
        <AppIcon name="alignLeft" class="size-3.5" />
      </span>
      <span v-if="card.attachments.length" class="flex items-center gap-1" :title="`${card.attachments.length} attached`">
        <AppIcon name="paperclip" class="size-3.5" />
        {{ card.attachments.length }}
      </span>
      <span
        v-if="card.checklist.length"
        class="flex items-center gap-1"
        :class="{ 'text-emerald-400': checklistDone === card.checklist.length }"
      >
        <AppIcon name="listChecks" class="size-3.5" />
        {{ checklistDone }}/{{ card.checklist.length }}
      </span>
    </div>
  </div>
</template>
