<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import AppIcon from '@/components/ui/AppIcon.vue'
import { debounce } from '@/utils/debounce'
import { breadcrumbTrail } from '@/utils/paths'

// root: the first crumb (default: the home folder); nothing above it is shown.
// hrefFor: the link of a crumb (default: that folder in the file browser).
const props = defineProps<{ path: string; root?: string; hrefFor?: (path: string) => string }>()
const emit = defineEmits<{
  navigate: [path: string]
  dropToFolder: [destPath: string, paths: string[]]
}>()

const router = useRouter()

function crumbHref(path: string): string {
  if (props.hrefFor) return props.hrefFor(path)
  return router.resolve({ query: path ? { path } : {} }).href
}

const navRef = ref<HTMLElement | null>(null)
const dotsBtnRef = ref<HTMLButtonElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
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
  const input = editInput.value
  if (input) {
    input.focus()
    input.setSelectionRange(input.value.length, input.value.length)
  }
}

function confirmEdit() {
  const path = editValue.value.trim().replace(/^\/+/, '').replace(/\/+$/, '').replace(/\/+/g, '/')
  editing.value = false
  emit('navigate', path)
}

function cancelEdit() {
  editing.value = false
}

const allCrumbs = computed(() => breadcrumbTrail(props.path, props.root))

const hiddenCrumbs = computed(() => allCrumbs.value.slice(0, hiddenCount.value))
const shownCrumbs = computed(() => allCrumbs.value.slice(hiddenCount.value))

// Collapse crumbs from the front using actual DOM overflow measurement.
// Priority: show as much of the last crumb (current item) as possible — keep
// hiding parents until the last crumb is no longer truncated or all parents
// are hidden. All iterations run as microtasks before the browser paints.
async function recalculate() {
  if (recalcRunning) { recalcQueued = true; return }
  recalcRunning = true

  while (true) {
    recalcQueued = false
    const nav = navRef.value
    if (!nav) break

    hiddenCount.value = 0
    await nextTick()

    // Hide all but the last crumb at most
    const maxHide = Math.max(0, allCrumbs.value.length - 1)
    while (hiddenCount.value < maxHide) {
      const links = nav.querySelectorAll<HTMLElement>('a')
      const lastLink = links[links.length - 1]
      // Stop once the last crumb fits without truncation
      if (!lastLink || lastLink.scrollWidth <= lastLink.clientWidth + 1) break
      hiddenCount.value++
      await nextTick()
    }

    if (!recalcQueued) break
  }

  recalcRunning = false
}

const scheduleRecalculate = debounce(recalculate, 60)

watch(
  navRef,
  (el, _prev, onCleanup) => {
    if (!el) return
    const ro = new ResizeObserver(scheduleRecalculate)
    ro.observe(el)
    onCleanup(() => {
      ro.disconnect()
      scheduleRecalculate.cancel()
    })
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
  if (dropdownRef.value?.contains(target)) return
  closeDropdown()
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') { closeDropdown(); cancelEdit() }
}

// Reposition/close dropdown on scroll or resize
function handleScrollResize() {
  if (dropdownOpen.value) closeDropdown()
}

// ── Breadcrumb drop targets ───────────────────────────────────────────────────
const dragOverCrumb = ref<string | null>(null)

function isCurrent(crumb: { path: string }): boolean {
  const crumbs = allCrumbs.value
  return crumb.path === crumbs[crumbs.length - 1]?.path
}

function onCrumbDragOver(e: DragEvent, crumb: { path: string }) {
  if (isCurrent(crumb)) return
  if (!e.dataTransfer?.types.includes('application/x-files-internal')) return
  e.preventDefault()
  e.dataTransfer.dropEffect = 'move'
  dragOverCrumb.value = crumb.path
}

function onCrumbDragLeave(e: DragEvent, crumb: { path: string }) {
  const related = e.relatedTarget as Node | null
  const el = e.currentTarget as HTMLElement
  if (!related || !el.contains(related)) {
    if (dragOverCrumb.value === crumb.path) dragOverCrumb.value = null
  }
}

function onCrumbDrop(e: DragEvent, crumb: { path: string }) {
  if (isCurrent(crumb)) return
  e.preventDefault()
  dragOverCrumb.value = null
  const raw = e.dataTransfer?.getData('application/x-files-internal')
  if (!raw) return
  const paths: string[] = JSON.parse(raw)
  emit('dropToFolder', crumb.path, paths)
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
  scheduleRecalculate.cancel()
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
    <nav v-else ref="navRef" class="w-full flex items-center gap-1 text-sm min-w-0 overflow-hidden">
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
          ref="dropdownRef"
          class="fixed z-[200] min-w-[10rem] max-h-60 overflow-y-auto bg-zinc-800 border border-zinc-700 rounded-lg shadow-xl py-1"
          :style="{ top: dropdownTop + 'px', left: dropdownLeft + 'px' }"
        >
          <a
            v-for="crumb in hiddenCrumbs"
            :key="crumb.path"
            :href="crumbHref(crumb.path)"
            :class="[
              'w-full text-left px-3 py-2.5 text-sm text-zinc-300 hover:text-white transition-colors flex items-center gap-2',
              dragOverCrumb === crumb.path ? 'bg-blue-800/60' : 'hover:bg-zinc-700',
            ]"
            @click.prevent="emit('navigate', crumb.path); closeDropdown()"
            @dragover="onCrumbDragOver($event, crumb)"
            @dragleave="onCrumbDragLeave($event, crumb)"
            @drop="onCrumbDrop($event, crumb)"
          >
            <AppIcon v-if="crumb.isHome" name="folder" class="size-3.5 shrink-0 text-zinc-400" />
            <span class="truncate">{{ crumb.label }}</span>
          </a>
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
      <a
        v-if="crumb.isHome"
        :href="crumbHref(crumb.path)"
        :class="[
          'shrink-0 min-h-[2.5rem] flex items-center gap-1.5 px-2 rounded transition-colors',
          !isCurrent(crumb) && dragOverCrumb === crumb.path
            ? 'bg-blue-800/50 text-white'
            : 'text-zinc-400 hover:text-white hover:bg-zinc-800/70',
        ]"
        @click.prevent="emit('navigate', crumb.path)"
        @dragover="onCrumbDragOver($event, crumb)"
        @dragleave="onCrumbDragLeave($event, crumb)"
        @drop="onCrumbDrop($event, crumb)"
      >
        <AppIcon name="folder" class="size-4 shrink-0" />
        <span>{{ crumb.label }}</span>
      </a>

      <!-- Segment -->
      <a
        v-else
        :href="crumbHref(crumb.path)"
        :class="[
          'min-h-[2.5rem] flex items-center px-2 rounded whitespace-nowrap transition-colors',
          idx === shownCrumbs.length - 1 ? 'min-w-0 overflow-hidden text-ellipsis' : 'shrink-0',
          !isCurrent(crumb) && dragOverCrumb === crumb.path
            ? 'bg-blue-800/50 text-white'
            : idx === shownCrumbs.length - 1
              ? 'text-zinc-100 hover:text-white hover:bg-zinc-800/70'
              : 'text-zinc-400 hover:text-white hover:bg-zinc-800/70',
        ]"
        @click.prevent="emit('navigate', crumb.path)"
        @dragover="onCrumbDragOver($event, crumb)"
        @dragleave="onCrumbDragLeave($event, crumb)"
        @drop="onCrumbDrop($event, crumb)"
      >
        {{ crumb.label }}
      </a>
    </template>

    <!-- Fills remaining row width so clicking empty space triggers enterEdit -->
    <div class="flex-1 min-w-2 self-stretch" @click="enterEdit" />
    </nav>

  </div>
</template>
