<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'

// A small centred dialog (confirmations, a name to type). It's marked as a
// modal, so the preview behind it ignores keys, and Escape closes it.
const props = withDefaults(defineProps<{
  open: boolean
  width?: string // Tailwind width classes of the card
}>(), { width: 'w-72' })

const emit = defineEmits<{ close: [] }>()

const card = ref<HTMLElement | null>(null)

// Only the dialog on top closes (another one can open over this).
function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  const dialogs = document.querySelectorAll('[aria-modal="true"]')
  if (dialogs[dialogs.length - 1] !== card.value) return
  event.preventDefault()
  emit('close')
}

watch(() => props.open, (open) => {
  if (open) window.addEventListener('keydown', onKeydown)
  else window.removeEventListener('keydown', onKeydown)
}, { immediate: true })
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <!-- Above the preview and AppModal (z-50): dialogs are opened from inside them. -->
  <Transition name="fade">
    <div
      v-if="open"
      class="fixed inset-0 z-[60] flex items-center justify-center bg-black/50"
      @mousedown.self="emit('close')"
    >
      <div
        ref="card"
        role="dialog"
        aria-modal="true"
        :class="['bg-zinc-900 border border-zinc-700 rounded-xl p-5 shadow-2xl', width]"
      >
        <slot />
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
