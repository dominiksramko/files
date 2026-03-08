<script setup lang="ts">
import { useToast } from '@/composables/useToast'

const { toast } = useToast()
</script>

<template>
  <Teleport to="body">
    <Transition name="toast">
      <div
        v-if="toast"
        :class="[
          'fixed bottom-6 right-6 z-[9999] px-4 py-2.5 rounded-lg shadow-xl text-sm font-medium pointer-events-none whitespace-nowrap',
          toast.kind === 'success' ? 'bg-green-800 text-green-100'
          : toast.kind === 'error'   ? 'bg-red-800 text-red-100'
          :                            'bg-zinc-700 text-zinc-100',
        ]"
      >
        {{ toast.message }}
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.toast-enter-active,
.toast-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(8px);
}
</style>
