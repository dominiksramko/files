<script setup lang="ts">
import { ref } from 'vue'
import AppIcon from '@/components/ui/AppIcon.vue'

defineProps<{
  name: string
  template: string
  active?: boolean
}>()

const emit = defineEmits<{
  select: []
  delete: []
}>()

const confirming = ref(false)

function requestDelete() {
  confirming.value = true
}

function confirmDelete() {
  confirming.value = false
  emit('delete')
}

function cancelDelete() {
  confirming.value = false
}
</script>

<template>
  <div
    class="w-full flex items-center rounded-lg border-l-2 transition-colors"
    :class="active
      ? 'border-blue-500 bg-zinc-800 text-zinc-100'
      : 'border-transparent text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/50'"
  >
    <button
      type="button"
      class="flex-1 text-left px-3 py-2 text-sm transition-colors"
      :title="template"
      @click="$emit('select')"
    >
      {{ name }}
    </button>

    <!-- Normal: delete trigger -->
    <button
      v-if="!confirming"
      type="button"
      class="px-3 py-2 text-zinc-600 hover:text-red-400 transition-colors shrink-0"
      title="Delete preset"
      @click="requestDelete"
    >
      <AppIcon name="x" class="size-3.5" />
    </button>

    <!-- Confirming: inline "Sure?" -->
    <template v-else>
      <span class="text-xs text-zinc-500 select-none">Sure?</span>
      <button
        type="button"
        class="px-2 py-2 text-red-400 hover:text-red-300 transition-colors text-xs font-medium"
        @click="confirmDelete"
      >Yes</button>
      <button
        type="button"
        class="px-2 py-2 text-zinc-500 hover:text-zinc-300 transition-colors"
        @click="cancelDelete"
      >
        <AppIcon name="x" class="size-3.5" />
      </button>
    </template>
  </div>
</template>
