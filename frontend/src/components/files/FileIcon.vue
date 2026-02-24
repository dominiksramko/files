<script setup lang="ts">
import { computed } from 'vue'
import { fileCategory } from '@/utils/formatters'

const props = defineProps<{ mime: string; name: string; isDir: boolean }>()

const category = computed(() => (props.isDir ? 'folder' : fileCategory(props.mime, props.name)))

const iconColor = computed(() => {
  switch (category.value) {
    case 'folder':
      return 'text-yellow-400'
    case 'image':
      return 'text-green-400'
    case 'video':
      return 'text-purple-400'
    case 'audio':
      return 'text-pink-400'
    case 'pdf':
      return 'text-red-400'
    case 'office':
      return 'text-blue-400'
    case 'archive':
      return 'text-orange-400'
    case 'text':
      return 'text-zinc-300'
    default:
      return 'text-zinc-500'
  }
})
</script>

<template>
  <svg
    :class="['size-5 shrink-0', iconColor]"
    fill="currentColor"
    viewBox="0 0 24 24"
    aria-hidden="true"
  >
    <!-- Folder -->
    <path
      v-if="category === 'folder'"
      d="M2 6a2 2 0 012-2h5l2 2h9a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2V6z"
    />
    <!-- Image -->
    <path
      v-else-if="category === 'image'"
      d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"
      stroke="currentColor"
      stroke-width="1.5"
      fill="none"
      stroke-linecap="round"
      stroke-linejoin="round"
    />
    <!-- Video -->
    <path
      v-else-if="category === 'video'"
      d="M15 10l4.553-2.276A1 1 0 0121 8.723v6.554a1 1 0 01-1.447.894L15 14M3 8a2 2 0 012-2h8a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2V8z"
      stroke="currentColor"
      stroke-width="1.5"
      fill="none"
      stroke-linecap="round"
      stroke-linejoin="round"
    />
    <!-- Audio -->
    <path
      v-else-if="category === 'audio'"
      d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3"
      stroke="currentColor"
      stroke-width="1.5"
      fill="none"
      stroke-linecap="round"
      stroke-linejoin="round"
    />
    <!-- Archive -->
    <path
      v-else-if="category === 'archive'"
      d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4"
      stroke="currentColor"
      stroke-width="1.5"
      fill="none"
      stroke-linecap="round"
      stroke-linejoin="round"
    />
    <!-- PDF -->
    <path
      v-else-if="category === 'pdf'"
      d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"
      stroke="currentColor"
      stroke-width="1.5"
      fill="none"
      stroke-linecap="round"
      stroke-linejoin="round"
    />
    <!-- Office / Text / Other: generic document -->
    <path
      v-else
      d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"
      stroke="currentColor"
      stroke-width="1.5"
      fill="none"
      stroke-linecap="round"
      stroke-linejoin="round"
    />
  </svg>
</template>
