<script setup lang="ts">
defineProps<{
  variant?: 'primary' | 'destructive' | 'secondary' | 'ghost'
  disabled?: boolean
  type?: 'button' | 'submit' | 'reset'
}>()
</script>

<template>
  <button
    :type="type ?? 'button'"
    :disabled="disabled"
    :class="[
      'px-3 py-1.5 text-sm rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500',
      variant === 'primary'     && 'bg-blue-600 hover:bg-blue-500 text-white',
      variant === 'destructive' && 'bg-red-700 hover:bg-red-600 text-white',
      variant === 'secondary'   && 'bg-zinc-700 hover:bg-zinc-600 text-white',
      (!variant || variant === 'ghost') && 'text-zinc-400 hover:text-white hover:bg-zinc-800',
    ]"
  ><slot /></button>
</template>
