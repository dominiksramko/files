<script setup lang="ts">
import { ref, onMounted } from 'vue'
import type { User } from '@/types'
import { users as usersApi, adminCache } from '@/utils/api'
import { useToast } from '@/composables/useToast'
import UserFormModal from '@/components/admin/UserFormModal.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const { show: showToast } = useToast()

const userList = ref<User[]>([])
const loading = ref(true)
const error = ref('')

const formUser = ref<User | null>(null)
const showForm = ref(false)
const showClearDialog = ref(false)
const clearingThumbs = ref(false)

async function fetchUsers() {
  loading.value = true
  error.value = ''
  try {
    userList.value = await usersApi.list()
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : 'Failed to load users'
  } finally {
    loading.value = false
  }
}

function openCreate() {
  formUser.value = null
  showForm.value = true
}

function openEdit(u: User) {
  formUser.value = u
  showForm.value = true
}

async function deleteUser(u: User) {
  if (!confirm(`Delete user "${u.username}"?`)) return
  try {
    await usersApi.delete(u.id)
    await fetchUsers()
  } catch (e: unknown) {
    showToast(e instanceof Error ? e.message : 'Failed to delete', 'error')
  }
}

function onSaved() {
  showForm.value = false
  fetchUsers()
}

async function confirmClearCache() {
  showClearDialog.value = false
  clearingThumbs.value = true
  error.value = ''
  try {
    await adminCache.clearAll()
    showToast('Cache cleared', 'success')
  } catch (e: unknown) {
    showToast(e instanceof Error ? e.message : 'Failed to clear cache', 'error')
  } finally {
    clearingThumbs.value = false
  }
}

onMounted(fetchUsers)
</script>

<template>
  <div class="h-full flex flex-col">

    <!-- Header -->
    <div class="px-4 py-3 sm:px-6 sm:py-4 border-b border-zinc-800 shrink-0">
      <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <h1 class="text-lg font-semibold text-white">User Management</h1>
        <div class="flex items-center gap-2">
          <button
            :disabled="clearingThumbs"
            class="flex items-center gap-1.5 text-sm bg-zinc-700 hover:bg-zinc-600 text-white px-3 py-2 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            @click="showClearDialog = true"
          >
            <AppIcon name="trash" class="size-4" />
            {{ clearingThumbs ? 'Clearing…' : 'Clear cache' }}
          </button>
          <button
            class="flex items-center gap-1.5 text-sm bg-blue-600 hover:bg-blue-500 text-white px-3 py-2 rounded-lg transition-colors"
            @click="openCreate"
          >
            <AppIcon name="plus" class="size-4" />
            New user
          </button>
        </div>
      </div>
    </div>

    <!-- Error -->
    <div
      v-if="error"
      class="px-6 py-2 text-sm text-red-400 bg-red-950/30 border-b border-red-900 shrink-0"
    >
      {{ error }}
    </div>

    <!-- Loading -->
    <div v-if="loading" class="flex-1 flex items-center justify-center">
      <AppSpinner size="lg" class="text-blue-500" />
    </div>

    <!-- Content -->
    <div v-else class="flex-1 overflow-auto">

      <!-- Mobile: card list -->
      <div class="sm:hidden divide-y divide-zinc-800">
        <div
          v-for="u in userList"
          :key="u.id"
          class="px-4 py-3 flex items-center gap-3"
        >
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="font-medium text-white text-sm">{{ u.display_name || u.username }}</span>
              <span v-if="u.id === auth.user?.id" class="text-xs text-zinc-500">(you)</span>
              <span
                :class="[
                  'text-xs font-medium px-2 py-0.5 rounded-full',
                  u.is_admin
                    ? 'bg-blue-950 text-blue-300 border border-blue-900'
                    : 'bg-zinc-800 text-zinc-400 border border-zinc-700',
                ]"
              >{{ u.is_admin ? 'admin' : 'user' }}</span>
            </div>
            <p v-if="u.display_name" class="text-xs text-zinc-500 mt-0.5">{{ u.username }}</p>
            <p class="text-xs text-zinc-600 font-mono mt-0.5 truncate">{{ u.root_dir || '—' }}</p>
          </div>
          <div class="flex items-center gap-1 shrink-0">
            <button
              class="p-2 rounded hover:bg-zinc-700 text-zinc-400 hover:text-white transition-colors"
              title="Edit"
              @click="openEdit(u)"
            >
              <AppIcon name="pencilSquare" class="size-4" />
            </button>
            <button
              :disabled="u.id === auth.user?.id"
              class="p-2 rounded hover:bg-red-950 text-zinc-400 hover:text-red-400 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
              title="Delete"
              @click="deleteUser(u)"
            >
              <AppIcon name="trash" class="size-4" />
            </button>
          </div>
        </div>
      </div>

      <!-- Desktop: table -->
      <table class="hidden sm:table w-full text-sm">
        <thead>
          <tr class="border-b border-zinc-800">
            <th class="text-left px-6 pb-3 pt-4 font-medium text-zinc-400">Username</th>
            <th class="text-left px-6 pb-3 pt-4 font-medium text-zinc-400">Root directory</th>
            <th class="text-left px-6 pb-3 pt-4 font-medium text-zinc-400 w-20">Role</th>
            <th class="w-24 pb-3 pt-4" />
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="u in userList"
            :key="u.id"
            class="border-b border-zinc-800/50 hover:bg-zinc-800/30 transition-colors"
          >
            <td class="py-3 px-6">
              <p class="font-medium text-white">
                {{ u.display_name || u.username }}
                <span v-if="u.id === auth.user?.id" class="ml-1.5 text-xs text-zinc-500">(you)</span>
              </p>
              <p v-if="u.display_name" class="text-xs text-zinc-500">{{ u.username }}</p>
            </td>
            <td class="py-3 px-6 text-zinc-400 font-mono text-xs">{{ u.root_dir || '—' }}</td>
            <td class="py-3 px-6">
              <span
                :class="[
                  'text-xs font-medium px-2 py-0.5 rounded-full',
                  u.is_admin
                    ? 'bg-blue-950 text-blue-300 border border-blue-900'
                    : 'bg-zinc-800 text-zinc-400 border border-zinc-700',
                ]"
              >
                {{ u.is_admin ? 'admin' : 'user' }}
              </span>
            </td>
            <td class="py-3 pr-6">
              <div class="flex items-center justify-end gap-1">
                <button
                  class="p-1.5 rounded hover:bg-zinc-700 text-zinc-400 hover:text-white transition-colors"
                  title="Edit"
                  @click="openEdit(u)"
                >
                  <AppIcon name="pencilSquare" class="size-4" />
                </button>
                <button
                  :disabled="u.id === auth.user?.id"
                  class="p-1.5 rounded hover:bg-red-950 text-zinc-400 hover:text-red-400 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                  title="Delete"
                  @click="deleteUser(u)"
                >
                  <AppIcon name="trash" class="size-4" />
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Clear cache dialog -->
    <Transition name="fade">
      <div
        v-if="showClearDialog"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
        @click.self="showClearDialog = false"
      >
        <div class="bg-zinc-900 border border-zinc-700 rounded-xl p-5 shadow-2xl w-72">
          <h3 class="text-sm font-semibold text-zinc-100 mb-3">Clear cache?</h3>
          <p class="text-xs text-zinc-400 mb-4">
            All generated thumbnails, image previews and HLS video segments will be deleted.
          </p>
          <div class="flex gap-2 justify-end">
            <button
              class="px-3 py-1.5 text-sm text-zinc-400 hover:text-white transition-colors rounded-lg hover:bg-zinc-800"
              @click="showClearDialog = false"
            >Cancel</button>
            <button
              class="px-3 py-1.5 text-sm bg-red-700 hover:bg-red-600 text-white rounded-lg transition-colors"
              @click="confirmClearCache"
            >Clear cache</button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- Form modal -->
    <UserFormModal
      v-if="showForm"
      :user="formUser"
      @close="showForm = false"
      @saved="onSaved"
    />
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
