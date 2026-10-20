<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useLayoutStore } from '@/stores/layout'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppDropdown from '@/components/ui/AppDropdown.vue'

const auth = useAuthStore()
const layout = useLayoutStore()
const router = useRouter()

async function logout() {
  await auth.logout().catch(() => { /* logged out locally anyway */ })
  router.push('/login')
}
</script>

<template>
  <header class="h-14 bg-zinc-900 border-b border-zinc-800 flex items-center px-4 gap-2 sm:gap-4 shrink-0">
    <!-- Logo: hidden when a view has registered a center component -->
    <RouterLink
      v-show="!layout.headerBreadcrumbsActive"
      to="/"
      class="flex items-center gap-2 font-semibold text-white text-sm shrink-0"
    >
      <AppIcon name="folder" class="size-5 text-blue-500" />
      files
    </RouterLink>

    <!-- Center slot: views register a component here via the layout store -->
    <component
      :is="layout.headerCenter"
      v-if="layout.headerCenter"
      class="flex-1 min-w-0 h-full"
    />

    <!-- Spacer when no center component is registered -->
    <div v-show="!layout.headerBreadcrumbsActive" class="flex-1" />

    <!-- User menu -->
    <AppDropdown align="right">
      <template #trigger="{ toggle }">
        <button
          class="flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm text-zinc-300 hover:text-white hover:bg-zinc-800 transition-colors"
          @click="toggle"
        >
          <span class="size-6 rounded-full bg-blue-600 flex items-center justify-center text-xs font-semibold text-white uppercase shrink-0">
            {{ (auth.user?.display_name || auth.user?.username)?.[0] }}
          </span>
          <span class="hidden sm:inline">{{ auth.user?.display_name || auth.user?.username }}</span>
          <AppIcon name="chevronDown" class="hidden sm:inline size-4" />
        </button>
      </template>

      <template #default>
        <RouterLink
          to="/settings"
          class="flex items-center gap-2 px-4 py-2 text-sm text-zinc-300 hover:text-white hover:bg-zinc-800 transition-colors"
        >
          <AppIcon name="cog" class="size-4" />
          Settings
        </RouterLink>
        <hr class="border-zinc-700 my-1" />
        <template v-if="auth.isAdmin">
          <RouterLink
            to="/admin"
            class="flex items-center gap-2 px-4 py-2 text-sm text-zinc-300 hover:text-white hover:bg-zinc-800 transition-colors"
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
      </template>
    </AppDropdown>
  </header>
</template>
