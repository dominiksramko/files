<script setup lang="ts">
import { ref, onMounted } from 'vue'
import type { User } from '@/types'
import { users as usersApi, thumbnails } from '@/utils/api'
import UserFormModal from '@/components/admin/UserFormModal.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const userList = ref<User[]>([])
const loading = ref(true)
const error = ref('')

const formUser = ref<User | null>(null)  // null = create mode, User = edit mode
const showForm = ref(false)
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
    error.value = e instanceof Error ? e.message : 'Failed to delete'
  }
}

function onSaved() {
  showForm.value = false
  fetchUsers()
}

async function clearThumbnails() {
  if (!confirm('Clear all thumbnail cache? This will delete all generated thumbnails.')) return
  clearingThumbs.value = true
  error.value = ''
  try {
    await thumbnails.clearAll()
    alert('Thumbnail cache cleared successfully')
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : 'Failed to clear thumbnails'
  } finally {
    clearingThumbs.value = false
  }
}

onMounted(fetchUsers)
</script>

<template>
  <div class="h-full flex flex-col">
    <!-- Header -->
    <div class="px-6 py-4 border-b border-zinc-800 flex items-center justify-between shrink-0">
      <h1 class="text-lg font-semibold text-white">User Management</h1>
      <div class="flex items-center gap-2">
        <button
          :disabled="clearingThumbs"
          class="flex items-center gap-1.5 text-sm bg-zinc-700 hover:bg-zinc-600 text-white px-4 py-2 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          @click="clearThumbnails"
        >
          <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
          </svg>
          {{ clearingThumbs ? 'Clearing...' : 'Clear thumbnails' }}
        </button>
        <button
          class="flex items-center gap-1.5 text-sm bg-blue-600 hover:bg-blue-700 text-white px-4 py-2 rounded-lg transition-colors"
          @click="openCreate"
        >
          <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
          New user
        </button>
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

    <!-- Table -->
    <div v-else class="flex-1 overflow-auto px-6 py-4">
      <table class="w-full text-sm">
        <thead>
          <tr class="border-b border-zinc-800">
            <th class="text-left pb-3 font-medium text-zinc-400">Username</th>
            <th class="text-left pb-3 font-medium text-zinc-400">Root directory</th>
            <th class="text-left pb-3 font-medium text-zinc-400 w-20">Role</th>
            <th class="w-24 pb-3" />
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="u in userList"
            :key="u.id"
            class="border-b border-zinc-800/50 hover:bg-zinc-800/30 transition-colors"
          >
            <td class="py-3 pr-4 font-medium text-white">
              {{ u.username }}
              <span v-if="u.id === auth.user?.id" class="ml-1.5 text-xs text-zinc-500">(you)</span>
            </td>
            <td class="py-3 pr-4 text-zinc-400 font-mono text-xs">
              {{ u.root_dir || '—' }}
            </td>
            <td class="py-3 pr-4">
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
            <td class="py-3">
              <div class="flex items-center justify-end gap-1">
                <button
                  class="p-1.5 rounded hover:bg-zinc-700 text-zinc-400 hover:text-white transition-colors"
                  title="Edit"
                  @click="openEdit(u)"
                >
                  <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                    />
                  </svg>
                </button>
                <button
                  :disabled="u.id === auth.user?.id"
                  class="p-1.5 rounded hover:bg-red-950 text-zinc-400 hover:text-red-400 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                  title="Delete"
                  @click="deleteUser(u)"
                >
                  <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      stroke-width="2"
                      d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                    />
                  </svg>
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Form modal -->
    <UserFormModal
      v-if="showForm"
      :user="formUser"
      @close="showForm = false"
      @saved="onSaved"
    />
  </div>
</template>
