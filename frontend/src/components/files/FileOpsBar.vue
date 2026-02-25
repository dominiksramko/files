<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{
  selected: string[]
  clipboard: { mode: 'copy' | 'cut'; paths: string[] } | null
}>()

const emit = defineEmits<{
  copy: []
  cut: []
  delete: []
  paste: []
  clearSelection: []
  clearClipboard: []
}>()

const confirmingDelete = ref(false)

function onDelete() {
  if (!confirmingDelete.value) {
    confirmingDelete.value = true
    return
  }
  confirmingDelete.value = false
  emit('delete')
}

function cancelDelete() {
  confirmingDelete.value = false
}
</script>

<template>
  <div
    v-if="selected.length > 0 || clipboard"
    class="shrink-0 px-4 py-2 bg-zinc-800 border-b border-zinc-700 flex items-center gap-2 flex-wrap text-sm"
  >
    <!-- Selection info + actions -->
    <template v-if="selected.length > 0">
      <span class="text-zinc-300 mr-1">
        {{ selected.length }} selected
      </span>

      <button
        class="px-2.5 py-1 rounded bg-zinc-700 hover:bg-zinc-600 text-zinc-200 transition-colors"
        title="Copy selected"
        @click="emit('copy')"
      >
        Copy
      </button>
      <button
        class="px-2.5 py-1 rounded bg-zinc-700 hover:bg-zinc-600 text-zinc-200 transition-colors"
        title="Cut selected"
        @click="emit('cut')"
      >
        Cut
      </button>

      <!-- Delete with inline confirmation -->
      <template v-if="confirmingDelete">
        <span class="text-red-400 text-xs">Delete {{ selected.length }} item{{ selected.length !== 1 ? 's' : '' }}?</span>
        <button
          class="px-2.5 py-1 rounded bg-red-700 hover:bg-red-600 text-white transition-colors"
          @click="onDelete"
        >
          Confirm
        </button>
        <button
          class="px-2.5 py-1 rounded bg-zinc-700 hover:bg-zinc-600 text-zinc-200 transition-colors"
          @click="cancelDelete"
        >
          Cancel
        </button>
      </template>
      <button
        v-else
        class="px-2.5 py-1 rounded bg-zinc-700 hover:bg-red-700 text-zinc-200 hover:text-white transition-colors"
        title="Delete selected"
        @click="onDelete"
      >
        Delete
      </button>

      <button
        class="ml-auto text-zinc-400 hover:text-white transition-colors p-1 rounded hover:bg-zinc-700"
        title="Clear selection"
        @click="emit('clearSelection')"
      >
        <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </template>

    <!-- Clipboard info + paste -->
    <template v-if="clipboard">
      <div
        v-if="selected.length > 0"
        class="w-px h-4 bg-zinc-600 mx-1"
      />
      <span class="text-zinc-400 italic text-xs">
        {{ clipboard.mode === 'copy' ? 'Copied' : 'Cut' }} {{ clipboard.paths.length }} item{{ clipboard.paths.length !== 1 ? 's' : '' }}
      </span>
      <button
        class="px-2.5 py-1 rounded bg-blue-700 hover:bg-blue-600 text-white transition-colors"
        title="Paste here"
        @click="emit('paste')"
      >
        Paste here
      </button>
      <button
        class="text-zinc-400 hover:text-white transition-colors p-1 rounded hover:bg-zinc-700"
        title="Clear clipboard"
        @click="emit('clearClipboard')"
      >
        <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </template>
  </div>
</template>
