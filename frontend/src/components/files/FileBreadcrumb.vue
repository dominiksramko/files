<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ path: string }>()
const emit = defineEmits<{ navigate: [path: string] }>()

const segments = computed(() => {
  if (!props.path) return []
  return props.path
    .split('/')
    .filter(Boolean)
    .map((seg, i, arr) => ({
      label: seg,
      path: arr.slice(0, i + 1).join('/'),
    }))
})
</script>

<template>
  <nav class="flex items-center gap-1 text-sm flex-wrap">
    <button
      class="flex items-center gap-1 text-zinc-400 hover:text-white transition-colors px-1 py-0.5 rounded"
      @click="emit('navigate', '')"
    >
      <svg class="size-4" fill="currentColor" viewBox="0 0 24 24">
        <path d="M2 6a2 2 0 012-2h5l2 2h9a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2V6z" />
      </svg>
      <span>Home</span>
    </button>

    <template v-for="seg in segments" :key="seg.path">
      <svg class="size-4 text-zinc-600 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
      </svg>
      <button
        class="text-zinc-400 hover:text-white transition-colors px-1 py-0.5 rounded max-w-xs truncate"
        @click="emit('navigate', seg.path)"
      >
        {{ seg.label }}
      </button>
    </template>
  </nav>
</template>
