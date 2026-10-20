<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { User } from '@/types'
import { users as usersApi } from '@/utils/api'
import AppButton from '@/components/ui/AppButton.vue'
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

// The form as it was opened, to tell whether closing would lose anything.
const fields = () => JSON.stringify([username.value, displayName.value, password.value, rootDir.value, isAdmin.value])
const opened = ref('')
const changed = computed(() => fields() !== opened.value)

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
    opened.value = fields()
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
  <AppModal :title="isEdit ? `Edit ${user?.username}` : 'Create user'" size="sm" :dirty="changed" @close="emit('close')">
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
          :autofocus="!isEdit"
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
          :autofocus="isEdit"
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
        <AppButton @click="emit('close')">Cancel</AppButton>
        <AppButton variant="primary" :disabled="loading" @click="save">
          <span class="flex items-center gap-1.5">
            <AppSpinner v-if="loading" size="sm" />
            {{ isEdit ? 'Save' : 'Create' }}
          </span>
        </AppButton>
      </div>
    </template>
  </AppModal>
</template>
