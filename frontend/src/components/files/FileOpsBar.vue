<script setup lang="ts">
import type { FileOp } from '@/types'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppDropdown from '@/components/ui/AppDropdown.vue'

defineProps<{
  ops: FileOp[]
  selectedCount: number
  clipboard: { mode: 'copy' | 'move'; paths: string[] } | null
}>()

const emit = defineEmits<{
  paste: []
  clearClipboard: []
  deselectAll: []
}>()
</script>

<template>
  <template v-if="selectedCount > 0">
    <!-- ••• button: mobile only — comes first so it's easiest to tap -->
    <AppDropdown class="sm:hidden">
      <template #trigger="{ toggle }">
        <button
          class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 transition-colors shrink-0"
          title="More actions"
          @click="toggle"
        >
          <AppIcon name="ellipsis" class="size-4" />
        </button>
      </template>

      <template #default>
        <template v-for="op in ops" :key="op.key">
          <button
            v-if="op.visible !== false"
            :disabled="op.disabled"
            :class="[
              'w-full text-left px-3 py-2.5 text-sm flex items-center gap-2.5 disabled:opacity-40 transition-colors',
              op.danger
                ? 'text-red-400 hover:bg-zinc-800'
                : 'text-zinc-300 hover:text-white hover:bg-zinc-800',
            ]"
            @click="op.action()"
          >
            <AppIcon :name="op.icon" class="size-4 shrink-0" />
            {{ op.label }}
          </button>
        </template>
      </template>
    </AppDropdown>

    <!-- Wider gap on mobile between ••• and deselect so they're not accidentally tapped together -->
    <div class="sm:hidden w-2 shrink-0" />

    <!-- Deselect + count: always visible -->
    <button
      class="flex items-center gap-1.5 text-sm text-zinc-200 hover:text-white shrink-0 pl-0.5 pr-2 py-1 rounded hover:bg-zinc-800 transition-colors"
      title="Clear selection"
      @click="emit('deselectAll')"
    >
      <AppIcon name="x" class="size-4 text-zinc-400" />
      {{ selectedCount }} selected
    </button>

    <div class="hidden sm:block w-px h-4 bg-zinc-700 mx-0.5 shrink-0" />

    <!-- Op buttons: inline on desktop only -->
    <template v-for="op in ops" :key="op.key">
      <button
        v-if="op.visible !== false"
        :disabled="op.disabled"
        :title="op.label"
        :class="[
          'hidden sm:flex p-1.5 rounded transition-colors shrink-0 disabled:opacity-40',
          op.danger
            ? 'text-zinc-400 hover:text-red-400 hover:bg-zinc-700'
            : 'text-zinc-400 hover:text-white hover:bg-zinc-700',
        ]"
        @click="op.action()"
      >
        <AppIcon :name="op.icon" class="size-4" />
      </button>
    </template>
  </template>

  <div v-if="selectedCount > 0 && clipboard" class="w-px h-4 bg-zinc-700 mx-0.5 shrink-0" />

  <template v-if="clipboard">
    <span class="text-xs text-zinc-400 italic min-w-0 truncate">
      {{ clipboard.mode === 'copy' ? 'Copying' : 'Moving' }}
      {{ clipboard.paths.length }} item{{ clipboard.paths.length !== 1 ? 's' : '' }}
    </span>
    <button
      class="flex items-center gap-1.5 px-2 py-1 rounded text-sm text-blue-400 hover:text-blue-300 hover:bg-zinc-700 transition-colors shrink-0"
      @click="emit('paste')"
    >
      <AppIcon name="clipboard" class="size-4" />
      Paste
    </button>
    <button
      class="flex items-center gap-1.5 px-2 py-1 rounded text-sm text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors shrink-0"
      @click="emit('clearClipboard')"
    >
      <AppIcon name="x" class="size-4" />
      Cancel
    </button>
  </template>
</template>
