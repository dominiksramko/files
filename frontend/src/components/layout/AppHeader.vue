<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import AppIcon from '@/components/ui/AppIcon.vue'

const auth = useAuthStore()
const router = useRouter()
const menuOpen = ref(false)
const menuRef = ref<HTMLElement | null>(null)

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
  <header class="h-14 bg-zinc-900 border-b border-zinc-800 flex items-center px-4 gap-4 shrink-0">
    <!-- Logo -->
    <RouterLink to="/" class="flex items-center gap-2 font-semibold text-white text-sm shrink-0">
      <AppIcon name="folder" class="size-5 text-blue-500" />
      files
    </RouterLink>

    <div class="flex-1" />

    <!-- User menu -->
    <div ref="menuRef" class="relative">
      <button
        class="flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm text-zinc-300 hover:text-white hover:bg-zinc-800 transition-colors"
        @click="menuOpen = !menuOpen"
      >
        <span
          class="size-6 rounded-full bg-blue-600 flex items-center justify-center text-xs font-semibold text-white uppercase shrink-0"
        >
          {{ (auth.user?.display_name || auth.user?.username)?.[0] }}
        </span>
        <span class="hidden sm:inline">{{ auth.user?.display_name || auth.user?.username }}</span>
        <AppIcon name="chevronDown" class="size-4" />
      </button>

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
            <AppIcon name="cog" class="size-4" />
            Settings
          </RouterLink>
          <hr class="border-zinc-700 my-1" />
          <template v-if="auth.isAdmin">
            <RouterLink
              to="/admin"
              class="flex items-center gap-2 px-4 py-2 text-sm text-zinc-300 hover:text-white hover:bg-zinc-800 transition-colors"
              @click="menuOpen = false"
            >
              <AppIcon name="users" class="size-4" />
              Admin
            </RouterLink>
            <hr class="border-zinc-700 my-1" />
          </template>
          <button
            class="w-full flex items-center gap-2 px-4 py-2 text-sm text-red-400 hover:text-red-300 hover:bg-zinc-800 transition-colors"
            @click="logout"
          >
            <AppIcon name="logout" class="size-4" />
            Log out
          </button>
        </div>
      </Transition>
    </div>
  </header>
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
