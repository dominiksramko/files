<script setup lang="ts">
defineProps<{
  title?: string
  size?: 'sm' | 'md' | 'lg' | 'xl'
}>()

const emit = defineEmits<{ close: [] }>()
</script>

<template>
  <!-- Backdrop -->
  <Teleport to="body">
    <div
      class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm"
      @click.self="emit('close')"
    >
      <div
        :class="[
          'bg-zinc-900 border border-zinc-700 rounded-xl shadow-2xl flex flex-col max-h-[90vh] w-full',
          size === 'sm'
            ? 'max-w-sm'
            : size === 'lg'
              ? 'max-w-2xl'
              : size === 'xl'
                ? 'max-w-4xl'
                : 'max-w-lg',
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
            <svg class="size-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
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
