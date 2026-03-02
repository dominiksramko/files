<script setup lang="ts">
defineProps<{ modelValue: boolean; indeterminate?: boolean }>()
defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>

<template>
  <button
    type="button"
    class="size-[18px] rounded border-2 flex items-center justify-center shrink-0 transition-colors"
    :class="modelValue
      ? 'bg-blue-500 border-blue-500'
      : indeterminate
        ? 'bg-zinc-700 border-zinc-500'
        : 'border-zinc-600'"
    @click.stop="$emit('update:modelValue', !modelValue)"
  >
    <svg v-if="modelValue" class="size-2.5 text-white" viewBox="0 0 10 8" fill="none">
      <path d="M1 4L3.5 6.5L9 1" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" />
    </svg>
    <svg v-else-if="indeterminate" class="size-2.5 text-white" viewBox="0 0 10 2" fill="none">
      <path d="M1 1H9" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" />
    </svg>
  </button>
</template>
