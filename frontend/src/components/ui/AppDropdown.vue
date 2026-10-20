<script setup lang="ts">
import { ref, onMounted, onUnmounted, useAttrs } from 'vue'

defineOptions({ inheritAttrs: false })

const attrs = useAttrs()

const props = withDefaults(defineProps<{
  align?: 'left' | 'right'
}>(), {
  align: 'left',
})

const triggerRef = ref<HTMLElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const isOpen = ref(false)
const top = ref(0)
const side = ref(0)

function open() {
  const rect = triggerRef.value?.getBoundingClientRect()
  if (!rect) return
  top.value = rect.bottom + 4
  side.value = props.align === 'right' ? window.innerWidth - rect.right : rect.left
  isOpen.value = true
}

function close() {
  isOpen.value = false
}

function toggle() {
  if (isOpen.value) close()
  else open()
}

function handleOutside(e: MouseEvent) {
  if (triggerRef.value?.contains(e.target as Node)) return
  if (dropdownRef.value?.contains(e.target as Node)) return
  close()
}

onMounted(() => document.addEventListener('mousedown', handleOutside))
onUnmounted(() => document.removeEventListener('mousedown', handleOutside))
</script>

<template>
  <div ref="triggerRef" v-bind="attrs">
    <slot name="trigger" :toggle="toggle" :is-open="isOpen" />
  </div>

  <Teleport to="body">
    <Transition name="dropdown">
      <div
        v-if="isOpen"
        ref="dropdownRef"
        class="fixed z-[200] min-w-[10rem] bg-zinc-900 border border-zinc-700 rounded-xl shadow-xl py-1"
        :style="align === 'right'
          ? { top: top + 'px', right: side + 'px' }
          : { top: top + 'px', left: side + 'px' }"
        @click="close"
      >
        <slot :close="close" />
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: opacity 0.1s ease, transform 0.1s ease;
}
.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
