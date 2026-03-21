<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import type { FileOp } from '@/types'
import AppIcon from '@/components/ui/AppIcon.vue'

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

const moreButtonRef = ref<HTMLButtonElement | null>(null)
const moreDropdownRef = ref<HTMLElement | null>(null)
const moreOpen = ref(false)
const dropdownTop = ref(0)
const dropdownLeft = ref(0)

function openMore() {
  const btn = moreButtonRef.value
  if (btn) {
    const rect = btn.getBoundingClientRect()
    dropdownTop.value = rect.bottom + 4
    dropdownLeft.value = rect.left
  }
  moreOpen.value = true
}

function closeMore() {
  moreOpen.value = false
}

function handleOutside(event: MouseEvent) {
  if (moreButtonRef.value?.contains(event.target as Node)) return
  if (moreDropdownRef.value?.contains(event.target as Node)) return
  closeMore()
}

onMounted(() => document.addEventListener('mousedown', handleOutside))
onUnmounted(() => document.removeEventListener('mousedown', handleOutside))
</script>

<template>
  <template v-if="selectedCount > 0">
    <!-- Deselect + count: always visible -->
    <button
      class="flex items-center gap-1.5 text-sm text-zinc-200 hover:text-white shrink-0 pl-0.5 pr-2 py-1 rounded hover:bg-zinc-800 transition-colors"
      title="Clear selection"
      @click="emit('deselectAll')"
    >
      <AppIcon name="x" class="size-4 text-zinc-400" />
      {{ selectedCount }} selected
    </button>

    <!-- ••• button: mobile only, opens dropdown with all ops -->
    <button
      ref="moreButtonRef"
      class="sm:hidden p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700 transition-colors shrink-0"
      title="More actions"
      @click="moreOpen ? closeMore() : openMore()"
    >
      <AppIcon name="ellipsis" class="size-4" />
    </button>

    <Teleport to="body">
      <div
        v-if="moreOpen"
        ref="moreDropdownRef"
        class="fixed z-[200] min-w-[10rem] bg-zinc-800 border border-zinc-700 rounded-lg shadow-xl py-1"
        :style="{ top: dropdownTop + 'px', left: dropdownLeft + 'px' }"
      >
        <template v-for="op in ops" :key="op.key">
          <button
            v-if="op.visible !== false"
            :disabled="op.disabled"
            :class="[
              'w-full text-left px-3 py-2.5 text-sm flex items-center gap-2.5 disabled:opacity-40 transition-colors',
              op.danger
                ? 'text-red-400 hover:bg-zinc-700'
                : 'text-zinc-300 hover:text-white hover:bg-zinc-700',
            ]"
            @click="op.action(); closeMore()"
          >
            <AppIcon :name="op.icon" class="size-4 shrink-0" />
            {{ op.label }}
          </button>
        </template>
      </div>
    </Teleport>

    <div class="w-px h-4 bg-zinc-700 mx-0.5 shrink-0" />

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
      class="p-1.5 rounded text-blue-400 hover:text-blue-300 hover:bg-zinc-700 transition-colors shrink-0"
      title="Paste here"
      @click="emit('paste')"
    >
      <AppIcon name="clipboard" class="size-4" />
    </button>
    <button
      class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors shrink-0"
      title="Clear clipboard"
      @click="emit('clearClipboard')"
    >
      <AppIcon name="x" class="size-4" />
    </button>
  </template>
</template>
