<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const menuOpen = ref(false)
const menuRef = ref<HTMLElement | null>(null)

// ── Search ──────────────────────────────────────────────────────────────────
const isFilesPage = computed(() => route.name === 'files')
const searchInput = ref('')
const searchExpanded = ref(false) // mobile expand state
const mobileInputRef = ref<HTMLInputElement | null>(null)

// Sync input from URL (handles back/forward navigation)
watch(
  () => route.query.q,
  (q) => {
    const val = (q as string) ?? ''
    if (searchInput.value !== val) searchInput.value = val
  },
  { immediate: true },
)

// Auto-focus input when mobile search expands
watch(searchExpanded, (val) => {
  if (val) nextTick(() => mobileInputRef.value?.focus())
})

function onSearchInput() {
  const current = (route.query.q as string) ?? ''
  if (current !== searchInput.value) {
    router.replace({ query: { ...route.query, q: searchInput.value || undefined } })
  }
}

function openSearch() {
  searchExpanded.value = true
}

function closeSearch() {
  searchExpanded.value = false
}

function clearSearch() {
  searchInput.value = ''
  searchExpanded.value = false
  router.replace({ query: { ...route.query, q: undefined } })
}

// ── User menu ───────────────────────────────────────────────────────────────
function logout() {
  menuOpen.value = false
  auth.logout()
  router.push('/login')
}

function handleClickOutside(e: MouseEvent) {
  if (menuRef.value && !menuRef.value.contains(e.target as Node)) {
    menuOpen.value = false
  }
}

onMounted(() => document.addEventListener('mousedown', handleClickOutside))
onUnmounted(() => document.removeEventListener('mousedown', handleClickOutside))
</script>

<template>
  <header class="h-14 bg-zinc-900 border-b border-zinc-800 flex items-center px-4 gap-4 shrink-0 relative">
    <!-- Logo -->
    <RouterLink to="/" class="flex items-center gap-2 font-semibold text-white text-sm shrink-0">
      <svg class="size-5 text-blue-500" fill="currentColor" viewBox="0 0 24 24">
        <path d="M2 6a2 2 0 012-2h5l2 2h9a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2V6z" />
      </svg>
      files
    </RouterLink>

    <!-- Desktop search (files page only, sm+) -->
    <div v-if="isFilesPage" class="hidden sm:flex flex-1 relative max-w-sm">
      <svg
        class="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-zinc-500 pointer-events-none"
        fill="none" viewBox="0 0 24 24" stroke="currentColor"
      >
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
          d="M21 21l-4.35-4.35M17 11A6 6 0 115 11a6 6 0 0112 0z" />
      </svg>
      <input
        v-model="searchInput"
        type="text"
        placeholder="Search files…"
        class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 text-sm rounded-lg pl-8 pr-7 py-1.5 border border-transparent focus:outline-none focus:border-zinc-600 transition-colors"
        @input="onSearchInput"
        @keydown.escape="clearSearch"
      />
      <button
        v-if="searchInput"
        class="absolute right-2 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-300"
        @click="clearSearch"
      >
        <svg class="size-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>

    <div class="flex-1" />

    <!-- Mobile search icon (files page only, before nav) -->
    <button
      v-if="isFilesPage"
      class="sm:hidden p-1.5 rounded-lg text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors"
      title="Search"
      @click="openSearch"
    >
      <svg class="size-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
          d="M21 21l-4.35-4.35M17 11A6 6 0 115 11a6 6 0 0112 0z" />
      </svg>
    </button>

    <!-- Nav -->
    <nav class="flex items-center gap-1">
      <RouterLink
        to="/"
        class="px-3 py-1.5 text-sm rounded-lg transition-colors text-zinc-400 hover:text-white hover:bg-zinc-800"
        active-class="!text-white !bg-zinc-800"
        exact-active-class="!text-white !bg-zinc-800"
      >
        Browse
      </RouterLink>
      <RouterLink
        v-if="auth.isAdmin"
        to="/admin"
        class="px-3 py-1.5 text-sm rounded-lg transition-colors text-zinc-400 hover:text-white hover:bg-zinc-800"
        active-class="!text-white !bg-zinc-800"
      >
        Admin
      </RouterLink>
    </nav>

    <!-- User menu -->
    <div ref="menuRef" class="relative">
      <button
        class="flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm text-zinc-300 hover:text-white hover:bg-zinc-800 transition-colors"
        @click="menuOpen = !menuOpen"
      >
        <span
          class="size-6 rounded-full bg-blue-600 flex items-center justify-center text-xs font-semibold text-white uppercase"
        >
          {{ auth.user?.username?.[0] }}
        </span>
        <span>{{ auth.user?.username }}</span>
        <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      <!-- Dropdown -->
      <Transition name="dropdown">
        <div
          v-if="menuOpen"
          class="absolute right-0 top-full mt-1 w-44 bg-zinc-900 border border-zinc-700 rounded-xl shadow-xl z-40 py-1"
        >
          <RouterLink
            to="/settings"
            class="flex items-center gap-2 px-4 py-2 text-sm text-zinc-300 hover:text-white hover:bg-zinc-800 transition-colors"
            @click="menuOpen = false"
          >
            <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
              />
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
              />
            </svg>
            Settings
          </RouterLink>
          <hr class="border-zinc-700 my-1" />
          <button
            class="w-full flex items-center gap-2 px-4 py-2 text-sm text-red-400 hover:text-red-300 hover:bg-zinc-800 transition-colors"
            @click="logout"
          >
            <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"
              />
            </svg>
            Log out
          </button>
        </div>
      </Transition>
    </div>

    <!-- Mobile expanded search overlay (covers header) -->
    <Transition name="search-expand">
      <div
        v-if="isFilesPage && searchExpanded"
        class="sm:hidden absolute inset-0 bg-zinc-900 flex items-center px-4 gap-3 z-20"
      >
        <div class="relative flex-1">
          <svg
            class="absolute left-2.5 top-1/2 -translate-y-1/2 size-4 text-zinc-500 pointer-events-none"
            fill="none" viewBox="0 0 24 24" stroke="currentColor"
          >
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
              d="M21 21l-4.35-4.35M17 11A6 6 0 115 11a6 6 0 0112 0z" />
          </svg>
          <input
            ref="mobileInputRef"
            v-model="searchInput"
            type="text"
            placeholder="Search files…"
            class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 text-sm rounded-lg pl-9 pr-8 py-2 border border-transparent focus:outline-none focus:border-zinc-600 transition-colors"
            @input="onSearchInput"
            @keydown.escape="closeSearch"
          />
          <button
            v-if="searchInput"
            class="absolute right-2.5 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-300"
            @click="clearSearch"
          >
            <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <button
          class="text-sm text-zinc-400 hover:text-white transition-colors shrink-0"
          @click="closeSearch"
        >
          Cancel
        </button>
      </div>
    </Transition>
  </header>
</template>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition:
    opacity 0.1s ease,
    transform 0.1s ease;
}
.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

.search-expand-enter-active,
.search-expand-leave-active {
  transition: opacity 0.15s ease;
}
.search-expand-enter-from,
.search-expand-leave-to {
  opacity: 0;
}
</style>
