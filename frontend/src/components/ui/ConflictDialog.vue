<script setup lang="ts">
import { useConflictDialog } from '@/composables/useConflictDialog'

const { visible, title, conflicts, choose } = useConflictDialog()
</script>

<template>
  <Transition name="fade">
    <div
      v-if="visible"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      @click.self="choose(null)"
    >
      <div class="bg-zinc-900 border border-zinc-700 rounded-xl p-5 w-80 shadow-2xl">
        <h3 class="text-sm font-semibold text-zinc-100 mb-1">{{ title }}</h3>
        <p class="text-xs text-zinc-400 mb-3">
          {{ conflicts.length }} file{{ conflicts.length !== 1 ? 's' : '' }} already exist in the destination.
        </p>
        <ul class="mb-4 space-y-1 max-h-36 overflow-y-auto">
          <li
            v-for="name in conflicts"
            :key="name"
            class="text-xs text-zinc-300 truncate bg-zinc-800 rounded px-2 py-1"
          >{{ name }}</li>
        </ul>
        <div class="flex gap-2 justify-end">
          <button
            class="px-3 py-1.5 text-sm text-zinc-400 hover:text-white transition-colors rounded-lg hover:bg-zinc-800"
            @click="choose(null)"
          >Cancel</button>
          <button
            class="px-3 py-1.5 text-sm bg-zinc-700 hover:bg-zinc-600 text-white rounded-lg transition-colors"
            @click="choose('replace')"
          >Replace</button>
          <button
            class="px-3 py-1.5 text-sm bg-blue-600 hover:bg-blue-500 text-white rounded-lg transition-colors"
            @click="choose('rename')"
          >Keep all</button>
        </div>
      </div>
    </div>
  </Transition>
</template>
