<script setup lang="ts">
import { ref, useId } from 'vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import { useEscapeToClose } from '@/composables/useEscapeToClose'
import { useModalFocus } from '@/composables/useModalFocus'

// A larger modal. The header has the close button first (on the left) and
// shows title, or the title slot (e.g. a breadcrumb) in its place; title also
// names the modal for screen readers. Keyboard focus moves into it and stays
// there until it closes (useModalFocus). On phones it fills the screen.
// closeOnEscape: Escape closes it (when no dialog is open over it). On by
// default; turn it off only where Escape must mean something else.
// dirty: closing would lose what was typed, so the close button, Escape and
// a click outside ask first.
// fixedHeight: always 90% of the screen tall, so it doesn't change size with
// its content.
const props = withDefaults(defineProps<{
  title?: string
  size?: 'sm' | 'md' | 'lg' | 'xl' | '2xl'
  closeOnEscape?: boolean
  fixedHeight?: boolean
  dirty?: boolean
}>(), { title: undefined, size: 'md', closeOnEscape: true })

const emit = defineEmits<{ close: [] }>()

// The larger sizes grow on big screens, where their content (long names,
// tables) would otherwise be cut short with room to spare. Below sm the
// modal fills the screen.
const widths = {
  sm: 'sm:max-w-sm',
  md: 'sm:max-w-lg',
  lg: 'sm:max-w-2xl xl:max-w-4xl',
  xl: 'sm:max-w-4xl xl:max-w-5xl 2xl:max-w-6xl',
  '2xl': 'sm:max-w-7xl 2xl:max-w-[96rem]',
}

const card = ref<HTMLElement | null>(null)
const titleId = useId()
const confirmingDiscard = ref(false)

function requestClose() {
  if (props.dirty) confirmingDiscard.value = true
  else emit('close')
}

function discard() {
  confirmingDiscard.value = false
  emit('close')
}

useEscapeToClose(card, () => props.closeOnEscape, requestClose)
useModalFocus(card, () => true)
</script>

<template>
  <!-- Backdrop -->
  <Teleport to="body">
    <div
      class="fixed inset-0 z-50 flex items-center justify-center sm:p-4 bg-black/50 backdrop-blur-sm"
      @mousedown.self="requestClose"
    >
      <div
        ref="card"
        role="dialog"
        aria-modal="true"
        tabindex="-1"
        data-modal-card
        :aria-labelledby="title && !$slots.title ? titleId : undefined"
        :aria-label="$slots.title ? title : undefined"
        :class="[
          'bg-zinc-900 shadow-2xl flex flex-col w-full h-dvh focus:outline-none sm:border sm:border-zinc-700 sm:rounded-xl',
          fixedHeight ? 'sm:h-[90vh]' : 'sm:h-auto sm:max-h-[90vh]',
          widths[size],
        ]"
      >
        <!-- Header: close first, where leaving is looked for -->
        <div v-if="title || $slots.title" class="flex items-center gap-2 px-5 py-4 border-b border-zinc-700 shrink-0">
          <button
            type="button"
            class="shrink-0 -my-1.5 -ml-2 p-1.5 rounded-lg text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors focus-visible:outline-2 focus-visible:outline-blue-500"
            aria-label="Close"
            title="Close"
            data-modal-close
            @click="requestClose"
          >
            <AppIcon name="x" class="size-5" />
          </button>
          <slot name="title">
            <h2 :id="titleId" class="min-w-0 truncate text-base font-semibold text-white">{{ title }}</h2>
          </slot>
        </div>

        <!-- Content -->
        <div class="overflow-y-auto flex-1 p-5">
          <slot />
        </div>

        <!-- Footer -->
        <div v-if="$slots.footer" class="px-5 py-4 border-t border-zinc-700 shrink-0">
          <slot name="footer" />
        </div>

        <!-- Inside the card, so it's the dialog on top -->
        <AppDialog :open="confirmingDiscard" @close="confirmingDiscard = false">
          <h3 class="text-sm font-semibold text-zinc-100 mb-3">Discard changes?</h3>
          <p class="text-xs text-zinc-400">What you changed here hasn't been saved and will be lost.</p>
          <template #actions>
            <AppButton autofocus @click="confirmingDiscard = false">Keep editing</AppButton>
            <AppButton variant="destructive" @click="discard">Discard</AppButton>
          </template>
        </AppDialog>
      </div>
    </div>
  </Teleport>
</template>
