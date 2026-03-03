<script setup lang="ts">
import { computed } from 'vue'
import { icons, type IconName } from '@/utils/icons'

const props = defineProps<{
  name: IconName
  strokeWidth?: number
}>()

const icon   = computed(() => icons[props.name])
const filled = computed(() => (icon.value as { filled?: boolean }).filled ?? false)
const sw     = computed(() => props.strokeWidth ?? 2)
</script>

<template>
  <svg
    viewBox="0 0 24 24"
    :fill="filled ? 'currentColor' : 'none'"
    :stroke="filled ? 'none' : 'currentColor'"
    aria-hidden="true"
  >
    <path
      :d="icon.d"
      :stroke-linecap="filled ? undefined : 'round'"
      :stroke-linejoin="filled ? undefined : 'round'"
      :stroke-width="filled ? undefined : sw"
    />
  </svg>
</template>
