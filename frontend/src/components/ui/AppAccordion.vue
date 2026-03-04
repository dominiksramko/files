<script setup lang="ts">
import { ref, provide } from 'vue'

const openId = ref<string | null>(null)

function toggle(id: string) {
  openId.value = openId.value === id ? null : id
}

provide('accordion', { openId, toggle })
</script>

<template>
  <div class="space-y-2">
    <slot />
  </div>
</template>
