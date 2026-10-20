<script setup lang="ts">
import AppSpinner from '@/components/ui/AppSpinner.vue'

// An area that's loading: a spinner bright enough to notice, what's happening
// (label), and below it anything more (a hint, progress, a Cancel button).
defineProps<{ label: string }>()
</script>

<template>
  <div class="flex flex-col items-center justify-center gap-3 py-12 text-center" role="status">
    <AppSpinner size="lg" class="text-blue-400" />
    <p class="text-sm text-zinc-200">{{ label }}</p>
    <slot />
  </div>
</template>
