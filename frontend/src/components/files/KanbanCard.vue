<script setup lang="ts">
import { computed } from 'vue'
import { dueStatus, formatDue, labelColor, type KanbanCard } from '@/utils/kanban'
import AppIcon from '@/components/ui/AppIcon.vue'

const props = defineProps<{ card: KanbanCard }>()

const emit = defineEmits<{ open: [] }>()

const checklistDone = computed(() => props.card.checklist.filter((item) => item.done).length)
const hasDescription = computed(() => props.card.description.trim() !== '')
const hasBadges = computed(() => props.card.due || hasDescription.value || props.card.checklist.length)

const dueClass = computed(() => {
  if (!props.card.due) return ''
  return {
    overdue:  'bg-red-500/20 text-red-300',
    today:    'bg-amber-500/20 text-amber-300',
    upcoming: 'text-zinc-400',
  }[dueStatus(props.card.due)]
})
</script>

<template>
  <div
    role="button"
    tabindex="0"
    class="cursor-pointer select-none rounded-lg border border-zinc-700/60 bg-zinc-800 px-3 py-2 transition-colors hover:border-zinc-500 focus:border-zinc-500 focus:outline-none"
    @click="emit('open')"
    @keydown.enter="emit('open')"
  >
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
      <span v-if="card.due" class="flex items-center gap-1 rounded px-1 py-0.5" :class="dueClass">
        <AppIcon name="calendar" class="size-3.5" />
        {{ formatDue(card.due) }}
      </span>
      <span v-if="hasDescription" title="Has a description">
        <AppIcon name="alignLeft" class="size-3.5" />
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
