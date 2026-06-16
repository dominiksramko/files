<script setup lang="ts">
import { onMounted, ref } from 'vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppCheckbox from '@/components/ui/AppCheckbox.vue'

defineProps<{
  done: boolean
  draggable?: boolean
}>()

const text = defineModel<string>('text', { required: true })

const emit = defineEmits<{
  toggle: []
  remove: []
  enter: []
  backspaceEmpty: []
}>()

const textareaRef = ref<HTMLTextAreaElement | null>(null)

// Keep the textarea height matched to its content (single line grows as it wraps).
function autoGrow() {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${el.scrollHeight}px`
}

function focus() {
  const el = textareaRef.value
  if (!el) return
  el.focus()
  const end = el.value.length
  el.setSelectionRange(end, end)
}

function onBackspace(event: KeyboardEvent) {
  if (text.value === '') {
    event.preventDefault()
    emit('backspaceEmpty')
  }
}

onMounted(autoGrow)
defineExpose({ focus })
</script>

<template>
  <li class="todo-row group flex items-start gap-1.5 rounded-lg bg-zinc-900 px-1 transition-colors hover:bg-zinc-800/40">
    <button
      v-if="draggable"
      type="button"
      class="todo-drag-handle shrink-0 cursor-grab pt-1.5 text-zinc-600 opacity-0 transition-opacity hover:text-zinc-300 group-hover:opacity-100 active:cursor-grabbing"
      title="Drag to reorder"
    >
      <AppIcon name="gripVertical" class="size-4" />
    </button>
    <span v-else class="w-4 shrink-0" />

    <AppCheckbox
      :model-value="done"
      class="mt-1.5"
      @update:model-value="emit('toggle')"
    />

    <textarea
      ref="textareaRef"
      v-model="text"
      rows="1"
      spellcheck="false"
      placeholder="List item"
      class="flex-1 resize-none overflow-hidden bg-transparent py-1 text-sm leading-relaxed placeholder-zinc-600 focus:outline-none"
      :class="done ? 'text-zinc-500 line-through' : 'text-zinc-100'"
      @input="autoGrow"
      @keydown.enter.prevent="emit('enter')"
      @keydown.backspace="onBackspace"
    />

    <button
      type="button"
      class="shrink-0 self-start rounded p-1 text-zinc-600 opacity-0 transition-all hover:bg-zinc-700 hover:text-red-400 group-hover:opacity-100"
      title="Delete"
      @click="emit('remove')"
    >
      <AppIcon name="trash" class="size-4" />
    </button>
  </li>
</template>
