<script setup lang="ts">
import { ref, watch } from 'vue'
import type { User } from '@/types'
import { users as usersApi } from '@/utils/api'
import AppModal from '@/components/ui/AppModal.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import AppCheckbox from '@/components/ui/AppCheckbox.vue'

const props = defineProps<{ user: User | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()

const username = ref('')
const displayName = ref('')
const password = ref('')
const rootDir = ref('')
const isAdmin = ref(false)
const loading = ref(false)
const error = ref('')

const isEdit = ref(false)

watch(
  () => props.user,
  (u) => {
    if (u) {
      isEdit.value = true
      username.value = u.username
      displayName.value = u.display_name ?? ''
      password.value = ''
      rootDir.value = u.root_dir
      isAdmin.value = u.is_admin
    } else {
      isEdit.value = false
      username.value = ''
      displayName.value = ''
      password.value = ''
      rootDir.value = ''
      isAdmin.value = false
    }
    error.value = ''
  },
  { immediate: true },
)

async function save() {
  loading.value = true
  error.value = ''
  try {
    if (isEdit.value && props.user) {
      await usersApi.update(props.user.id, {
        password: password.value || undefined,
        display_name: displayName.value,
        root_dir: rootDir.value || undefined,
        is_admin: isAdmin.value,
      })
    } else {
      await usersApi.create({
        username: username.value,
        password: password.value,
        display_name: displayName.value || undefined,
        root_dir: rootDir.value || undefined,
        is_admin: isAdmin.value,
      })
    }
    emit('saved')
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : 'Failed to save'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <AppModal :title="isEdit ? `Edit ${user?.username}` : 'Create user'" size="sm" @close="emit('close')">
    <form class="space-y-3" @submit.prevent="save">
      <!-- Error -->
      <div
        v-if="error"
        class="text-sm text-red-400 bg-red-950/50 border border-red-900 rounded-lg px-3 py-2"
      >
        {{ error }}
      </div>

      <div>
        <label class="block text-xs font-medium text-zinc-400 mb-1.5">Username</label>
        <input
          v-model="username"
          type="text"
          autocomplete="off"
          :disabled="isEdit"
          required
          class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-white placeholder-zinc-500 focus:outline-none focus:border-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
          placeholder="username"
        />
      </div>

      <div>
        <label class="block text-xs font-medium text-zinc-400 mb-1.5">Display name (optional)</label>
        <input
          v-model="displayName"
          type="text"
          class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-white placeholder-zinc-500 focus:outline-none focus:border-blue-500"
          placeholder="Alice Smith"
        />
      </div>

      <div>
        <label class="block text-xs font-medium text-zinc-400 mb-1.5">
          Password{{ isEdit ? ' (leave blank to keep current)' : '' }}
        </label>
        <input
          v-model="password"
          type="password"
          autocomplete="new-password"
          :required="!isEdit"
          class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-white placeholder-zinc-500 focus:outline-none focus:border-blue-500"
          placeholder="••••••••"
        />
      </div>

      <div>
        <label class="block text-xs font-medium text-zinc-400 mb-1.5">
          Root directory (leave blank for default)
        </label>
        <input
          v-model="rootDir"
          type="text"
          class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-white placeholder-zinc-500 focus:outline-none focus:border-blue-500"
          placeholder="/data/users/alice"
        />
      </div>

      <label class="flex items-center gap-2 cursor-pointer select-none">
        <AppCheckbox v-model="isAdmin" />
        <span class="text-sm text-zinc-300">Admin</span>
      </label>
    </form>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button
          class="text-sm text-zinc-400 hover:text-white px-4 py-2 rounded-lg hover:bg-zinc-800 transition-colors"
          @click="emit('close')"
        >
          Cancel
        </button>
        <button
          :disabled="loading"
          class="flex items-center gap-1.5 text-sm bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed text-white px-4 py-2 rounded-lg transition-colors"
          @click="save"
        >
          <AppSpinner v-if="loading" size="sm" />
          {{ isEdit ? 'Save' : 'Create' }}
        </button>
      </div>
    </template>
  </AppModal>
</template>
