<script setup lang="ts">
import AppIcon from '@/components/ui/AppIcon.vue'

defineProps<{
  title?: string
  size?: 'sm' | 'md' | 'lg' | 'xl' | '2xl'
}>()

const emit = defineEmits<{ close: [] }>()
</script>

<template>
  <!-- Backdrop -->
  <Teleport to="body">
    <div
      class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
      @mousedown.self="emit('close')"
    >
      <div
        role="dialog"
        aria-modal="true"
        :class="[
          'bg-zinc-900 border border-zinc-700 rounded-xl shadow-2xl flex flex-col max-h-[90vh] w-full',
          { sm: 'max-w-sm', md: 'max-w-lg', lg: 'max-w-2xl', xl: 'max-w-4xl', '2xl': 'max-w-7xl' }[size ?? 'md'],
        ]"
      >
        <!-- Header -->
        <div v-if="title" class="flex items-center justify-between px-5 py-4 border-b border-zinc-700 shrink-0">
          <h2 class="text-base font-semibold text-white">{{ title }}</h2>
          <button
            class="text-zinc-400 hover:text-white transition-colors"
            @click="emit('close')"
            aria-label="Close"
          >
            <AppIcon name="x" class="size-5" />
          </button>
        </div>

        <!-- Content -->
        <div class="overflow-y-auto flex-1 p-5">
          <slot />
        </div>

        <!-- Footer -->
        <div v-if="$slots.footer" class="px-5 py-4 border-t border-zinc-700 shrink-0">
          <slot name="footer" />
        </div>
      </div>
    </div>
  </Teleport>
</template>
