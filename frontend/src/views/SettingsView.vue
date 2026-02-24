<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { auth as authApi } from '@/utils/api'
import AppSpinner from '@/components/ui/AppSpinner.vue'

const authStore = useAuthStore()

const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const loading = ref(false)
const error = ref('')
const success = ref(false)

async function changePassword() {
  if (newPassword.value !== confirmPassword.value) {
    error.value = 'New passwords do not match'
    return
  }
  if (newPassword.value.length < 4) {
    error.value = 'Password must be at least 4 characters'
    return
  }

  loading.value = true
  error.value = ''
  success.value = false

  try {
    await authApi.changePassword(currentPassword.value, newPassword.value)
    success.value = true
    currentPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : 'Failed to change password'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="h-full overflow-auto">
    <div class="max-w-lg mx-auto px-6 py-8 space-y-8">
      <!-- Account info -->
      <div>
        <h1 class="text-lg font-semibold text-white mb-4">Settings</h1>
        <div class="bg-zinc-900 border border-zinc-800 rounded-xl p-4 space-y-2">
          <div class="flex items-center gap-3">
            <span
              class="size-10 rounded-full bg-blue-600 flex items-center justify-center text-sm font-semibold text-white uppercase"
            >
              {{ authStore.user?.username?.[0] }}
            </span>
            <div>
              <p class="font-medium text-white">{{ authStore.user?.username }}</p>
              <p class="text-xs text-zinc-500">
                {{ authStore.user?.is_admin ? 'Administrator' : 'User' }}
              </p>
            </div>
          </div>
          <div class="pt-2 border-t border-zinc-800">
            <p class="text-xs text-zinc-500 font-medium mb-0.5">Root directory</p>
            <p class="text-sm text-zinc-300 font-mono">{{ authStore.user?.root_dir || '—' }}</p>
          </div>
        </div>
      </div>

      <!-- Change password -->
      <div>
        <h2 class="text-sm font-semibold text-zinc-300 mb-3">Change password</h2>
        <form
          class="bg-zinc-900 border border-zinc-800 rounded-xl p-4 space-y-3"
          @submit.prevent="changePassword"
        >
          <div
            v-if="success"
            class="text-sm text-green-400 bg-green-950/50 border border-green-900 rounded-lg px-3 py-2"
          >
            Password updated successfully.
          </div>
          <div
            v-if="error"
            class="text-sm text-red-400 bg-red-950/50 border border-red-900 rounded-lg px-3 py-2"
          >
            {{ error }}
          </div>

          <div>
            <label class="block text-xs font-medium text-zinc-400 mb-1.5">Current password</label>
            <input
              v-model="currentPassword"
              type="password"
              required
              class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-white placeholder-zinc-500 focus:outline-none focus:border-blue-500"
            />
          </div>
          <div>
            <label class="block text-xs font-medium text-zinc-400 mb-1.5">New password</label>
            <input
              v-model="newPassword"
              type="password"
              required
              class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-white placeholder-zinc-500 focus:outline-none focus:border-blue-500"
            />
          </div>
          <div>
            <label class="block text-xs font-medium text-zinc-400 mb-1.5">Confirm new password</label>
            <input
              v-model="confirmPassword"
              type="password"
              required
              class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-white placeholder-zinc-500 focus:outline-none focus:border-blue-500"
            />
          </div>

          <div class="flex justify-end pt-1">
            <button
              type="submit"
              :disabled="loading"
              class="flex items-center gap-1.5 text-sm bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed text-white px-4 py-2 rounded-lg transition-colors"
            >
              <AppSpinner v-if="loading" size="sm" />
              {{ loading ? 'Saving…' : 'Update password' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>
