<script setup lang="ts">
import { onMounted, onUnmounted, ref, nextTick } from 'vue'

export interface ContextMenuItem {
  label?: string
  danger?: boolean
  separator?: boolean
  action?: () => void
}

const props = defineProps<{
  x: number
  y: number
  items: ContextMenuItem[]
}>()

const emit = defineEmits<{ close: [] }>()

const menuRef = ref<HTMLElement | null>(null)
const top = ref(props.y)
const left = ref(props.x)

function onMousedown(e: MouseEvent) {
  if (menuRef.value && !menuRef.value.contains(e.target as Node)) {
    emit('close')
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

onMounted(async () => {
  document.addEventListener('mousedown', onMousedown, true)
  document.addEventListener('keydown', onKeydown)
  await nextTick()
  if (menuRef.value) {
    const rect = menuRef.value.getBoundingClientRect()
    if (left.value + rect.width > window.innerWidth - 8) {
      left.value = Math.max(8, window.innerWidth - rect.width - 8)
    }
    if (top.value + rect.height > window.innerHeight - 8) {
      top.value = Math.max(8, window.innerHeight - rect.height - 8)
    }
  }
})

onUnmounted(() => {
  document.removeEventListener('mousedown', onMousedown, true)
  document.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div
    ref="menuRef"
    class="fixed z-[70] bg-zinc-900 border border-zinc-700 rounded-xl shadow-2xl py-1 min-w-44"
    :style="{ left: left + 'px', top: top + 'px' }"
  >
    <template v-for="(item, i) in items" :key="i">
      <div v-if="item.separator" class="my-1 border-t border-zinc-800" />
      <button
        v-else
        :class="[
          'w-full text-left px-3 py-1.5 text-sm transition-colors',
          item.danger
            ? 'text-red-400 hover:bg-red-950/60 hover:text-red-300'
            : 'text-zinc-200 hover:bg-zinc-800',
        ]"
        @click="item.action?.()"
      >
        {{ item.label }}
      </button>
    </template>
  </div>
</template>
