<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import AppIcon from '@/components/ui/AppIcon.vue'

const props = defineProps<{ path: string }>()
const emit = defineEmits<{ navigate: [path: string] }>()

const navRef = ref<HTMLElement | null>(null)
const dotsBtnRef = ref<HTMLButtonElement | null>(null)
const hiddenCount = ref(0)
const dropdownOpen = ref(false)
const dropdownTop = ref(0)
const dropdownLeft = ref(0)
let recalcRunning = false
let recalcQueued = false

// ── Dolphin-style path editing ────────────────────────────────────────────────
const editing   = ref(false)
const editValue = ref('')
const editInput = ref<HTMLInputElement | null>(null)

async function enterEdit() {
  if (editing.value) return
  closeDropdown()
  editValue.value = props.path
  editing.value = true
  await nextTick()
  editInput.value?.select()
}

function confirmEdit() {
  const path = editValue.value.trim().replace(/^\/+/, '').replace(/\/+$/, '').replace(/\/+/g, '/')
  editing.value = false
  emit('navigate', path)
}

function cancelEdit() {
  editing.value = false
}

const allCrumbs = computed(() => {
  const crumbs: { label: string; path: string; isHome: boolean }[] = [
    { label: 'Home', path: '', isHome: true },
  ]
  if (!props.path) return crumbs
  props.path
    .split('/')
    .filter(Boolean)
    .forEach((seg, i, arr) => {
      crumbs.push({ label: seg, path: arr.slice(0, i + 1).join('/'), isHome: false })
    })
  return crumbs
})

const hiddenCrumbs = computed(() => allCrumbs.value.slice(0, hiddenCount.value))
const shownCrumbs = computed(() => allCrumbs.value.slice(hiddenCount.value))

// Collapse crumbs from the front using actual DOM overflow measurement.
// All iterations run as microtasks before the browser paints — no visible flicker.
async function recalculate() {
  if (recalcRunning) { recalcQueued = true; return }
  recalcRunning = true

  while (true) {
    recalcQueued = false
    const nav = navRef.value
    if (!nav) break

    hiddenCount.value = 0
    await nextTick()

    // Always keep at least the last 2 crumbs visible
    const maxHide = Math.max(0, allCrumbs.value.length - 2)
    while (nav.scrollWidth > nav.clientWidth + 1 && hiddenCount.value < maxHide) {
      hiddenCount.value++
      await nextTick()
    }

    if (!recalcQueued) break
  }

  recalcRunning = false
}

watch(
  navRef,
  (el, _prev, onCleanup) => {
    if (!el) return
    const ro = new ResizeObserver(() => recalculate())
    ro.observe(el)
    onCleanup(() => ro.disconnect())
  },
  { flush: 'post' },
)

watch(() => props.path, () => {
  editing.value = false
  hiddenCount.value = 0
  nextTick(recalculate)
})

// Open dropdown and anchor it to the trigger button position
function openDropdown() {
  const btn = dotsBtnRef.value
  if (btn) {
    const r = btn.getBoundingClientRect()
    dropdownTop.value = r.bottom + 4
    dropdownLeft.value = r.left
  }
  dropdownOpen.value = true
}

function closeDropdown() {
  dropdownOpen.value = false
}

// Close on outside click or Escape
function handleOutside(e: MouseEvent) {
  const target = e.target as Node
  if (dotsBtnRef.value?.contains(target)) return
  closeDropdown()
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') { closeDropdown(); cancelEdit() }
}

// Reposition/close dropdown on scroll or resize
function handleScrollResize() {
  if (dropdownOpen.value) closeDropdown()
}

onMounted(() => {
  document.addEventListener('mousedown', handleOutside)
  document.addEventListener('keydown', handleKeydown)
  window.addEventListener('resize', handleScrollResize, { passive: true })
  window.addEventListener('scroll', handleScrollResize, { passive: true, capture: true })
})

onUnmounted(() => {
  document.removeEventListener('mousedown', handleOutside)
  document.removeEventListener('keydown', handleKeydown)
  window.removeEventListener('resize', handleScrollResize)
  window.removeEventListener('scroll', handleScrollResize, { capture: true })
})
</script>

<template>
  <!-- Outer wrapper fills the breadcrumb row; clicking empty space enters edit mode -->
  <div class="flex-1 min-w-0 h-full flex items-center" @click.self="enterEdit">

    <!-- Edit mode: inline path input -->
    <input
      v-if="editing"
      ref="editInput"
      v-model="editValue"
      type="text"
      class="flex-1 min-w-0 bg-transparent text-zinc-100 text-sm focus:outline-none font-mono py-1"
      spellcheck="false"
      @keydown.enter.prevent="confirmEdit"
      @keydown.escape.prevent="cancelEdit"
      @blur="cancelEdit"
    />

    <!-- Normal mode: breadcrumbs -->
    <nav v-else ref="navRef" class="flex items-center gap-1 text-sm min-w-0 overflow-hidden">
    <!-- "•••" trigger — Teleport escapes overflow:hidden so dropdown is never clipped -->
    <template v-if="hiddenCrumbs.length > 0">
      <button
        ref="dotsBtnRef"
        class="shrink-0 min-w-[2.5rem] min-h-[2.5rem] flex items-center justify-center rounded text-zinc-500 hover:text-white hover:bg-zinc-800 transition-colors text-xs font-bold tracking-widest"
        @click.stop="dropdownOpen ? closeDropdown() : openDropdown()"
      >
        •••
      </button>
      <Teleport to="body">
        <div
          v-if="dropdownOpen"
          class="fixed z-[200] min-w-[10rem] max-h-60 overflow-y-auto bg-zinc-800 border border-zinc-700 rounded-lg shadow-xl py-1"
          :style="{ top: dropdownTop + 'px', left: dropdownLeft + 'px' }"
        >
          <button
            v-for="crumb in hiddenCrumbs"
            :key="crumb.path"
            class="w-full text-left px-3 py-2.5 text-sm text-zinc-300 hover:text-white hover:bg-zinc-700 transition-colors flex items-center gap-2"
            @click="emit('navigate', crumb.path); closeDropdown()"
          >
            <AppIcon v-if="crumb.isHome" name="folder" class="size-3.5 shrink-0 text-zinc-400" />
            <span class="truncate">{{ crumb.label }}</span>
          </button>
        </div>
      </Teleport>
    </template>

    <!-- Visible crumbs -->
    <template v-for="(crumb, idx) in shownCrumbs" :key="crumb.path">
      <AppIcon
        v-if="idx > 0 || hiddenCrumbs.length > 0"
        name="chevronRight"
        class="size-4 text-zinc-600 shrink-0"
      />

      <!-- Home -->
      <button
        v-if="crumb.isHome"
        class="shrink-0 min-h-[2.5rem] flex items-center gap-1.5 px-2 rounded text-zinc-400 hover:text-white hover:bg-zinc-800/70 transition-colors"
        @click="emit('navigate', '')"
      >
        <AppIcon name="folder" class="size-4 shrink-0" />
        <span>Home</span>
      </button>

      <!-- Segment -->
      <button
        v-else
        :class="[
          'shrink-0 min-h-[2.5rem] flex items-center px-2 rounded whitespace-nowrap transition-colors hover:bg-zinc-800/70',
          idx === shownCrumbs.length - 1 ? 'text-zinc-100 hover:text-white' : 'text-zinc-400 hover:text-white',
        ]"
        @click="emit('navigate', crumb.path)"
      >
        {{ crumb.label }}
      </button>
    </template>

    <!-- Fills remaining row width so clicking empty space triggers enterEdit -->
    <div class="flex-1 min-w-2 self-stretch" @click="enterEdit" />
    </nav>

  </div>
</template>
