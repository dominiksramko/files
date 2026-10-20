<script setup lang="ts">
import { inject, computed, ref } from 'vue'
import type { Ref } from 'vue'
import AppIcon from './AppIcon.vue'

defineProps<{ title: string }>()

const id = ref(Math.random().toString(36).slice(2))
const accordion = inject<{ openId: Ref<string | null>; toggle: (id: string) => void }>('accordion')!
const isOpen = computed(() => accordion.openId.value === id.value)
</script>

<template>
  <div class="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
    <button
      type="button"
      class="w-full flex items-center justify-between px-4 py-3 text-sm font-semibold text-zinc-300 hover:text-white hover:bg-zinc-800/50 transition-colors"
      @click="accordion.toggle(id)"
    >
      {{ title }}
      <AppIcon
        name="chevronDown"
        class="size-4 text-zinc-500 transition-transform duration-200"
        :class="{ 'rotate-180': isOpen }"
      />
    </button>

    <div
      class="grid transition-[grid-template-rows] duration-200 ease-in-out"
      :class="isOpen ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'"
    >
      <div class="overflow-hidden">
        <div class="px-4 pb-4 pt-1 border-t border-zinc-800">
          <slot />
        </div>
      </div>
    </div>
  </div>
</template>
