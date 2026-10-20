<script setup lang="ts">
import { useConflictDialog } from '@/composables/useConflictDialog'
import AppButton from '@/components/ui/AppButton.vue'
import AppDialog from '@/components/ui/AppDialog.vue'

const { visible, title, conflicts, choose } = useConflictDialog()
</script>

<template>
  <AppDialog :open="visible" width="w-80" @close="choose(null)">
    <h3 class="text-sm font-semibold text-zinc-100 mb-1">{{ title }}</h3>
    <p class="text-xs text-zinc-400 mb-3">
      {{ conflicts.length }} file{{ conflicts.length !== 1 ? 's' : '' }} already exist in the destination.
    </p>
    <ul class="space-y-1 max-h-36 overflow-y-auto">
      <li
        v-for="name in conflicts"
        :key="name"
        class="text-xs text-zinc-300 truncate bg-zinc-800 rounded px-2 py-1"
      >{{ name }}</li>
    </ul>
    <template #actions>
      <AppButton @click="choose(null)">Cancel</AppButton>
      <AppButton variant="secondary" @click="choose('replace')">Replace</AppButton>
      <AppButton variant="primary" @click="choose('rename')">Keep all</AppButton>
    </template>
  </AppDialog>
</template>
