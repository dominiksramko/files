<script setup lang="ts">
import { ref, computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { auth as authApi } from '@/utils/api'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import AppAccordion from '@/components/ui/AppAccordion.vue'
import AppAccordionItem from '@/components/ui/AppAccordionItem.vue'
import { useToast } from '@/composables/useToast'
import type { SubtitlePrefs } from '@/types'

const authStore = useAuthStore()
const { show: showToast } = useToast()

// --- Subtitle prefs ---
const DEFAULT_SUBTITLE_PREFS: SubtitlePrefs = {
  color: '#ffffff', bg_opacity: 0.5, font_size: 100, text_style: 'drop-shadow', font_family: 'sans-serif',
}

const colorOptions = [
  { label: 'White',   value: '#ffffff' },
  { label: 'Black',   value: '#000000' },
  { label: 'Red',     value: '#ff0000' },
  { label: 'Green',   value: '#00ff00' },
  { label: 'Blue',    value: '#0000ff' },
  { label: 'Yellow',  value: '#ffff00' },
  { label: 'Magenta', value: '#ff00ff' },
  { label: 'Cyan',    value: '#00ffff' },
]
const bgOptions = [
  { label: 'None', value: 0 },
  { label: '25%',  value: 0.25 },
  { label: '33%',  value: 0.33 },
  { label: '50%',  value: 0.5 },
  { label: '66%',  value: 0.66 },
  { label: '75%',  value: 0.75 },
  { label: '100%', value: 1 },
]
const sizeOptions = [
  { label: 'Tiny',        value: 60 },
  { label: 'Small',       value: 75 },
  { label: 'Medium',      value: 100 },
  { label: 'Large',       value: 125 },
  { label: 'Extra Large', value: 150 },
]
const styleOptions = [
  { label: 'None',        value: 'none' },
  { label: 'Raised',      value: 'raised' },
  { label: 'Depressed',   value: 'depressed' },
  { label: 'Uniform',     value: 'uniform' },
  { label: 'Drop shadow', value: 'drop-shadow' },
]
const fontOptions = [
  { label: 'Sans-serif',  value: 'sans-serif' },
  { label: 'Serif',       value: 'serif' },
  { label: 'Monospaced',  value: 'monospace' },
  { label: 'Arial',       value: 'Arial, sans-serif' },
  { label: 'Verdana',     value: 'Verdana, sans-serif' },
  { label: 'Georgia',     value: 'Georgia, serif' },
  { label: 'Courier New', value: "'Courier New', monospace" },
  { label: 'Trebuchet',   value: "'Trebuchet MS', sans-serif" },
]

const SHADOW_MAP: Record<string, string> = {
  'none':        'none',
  'raised':      'rgb(0,0,0) 4px 4px 0px',
  'depressed':   'rgb(0,0,0) 4px 4px 0px, rgba(0,0,0,0.25) 0px 4px 4px',
  'uniform':     'rgb(0,0,0) -3px 0px, rgb(0,0,0) 0px 3px, rgb(0,0,0) 3px 0px, rgb(0,0,0) 0px -3px',
  'drop-shadow': 'rgba(0,0,0,0.75) 4px 4px 4px',
}

const localPrefs = ref<SubtitlePrefs>(
  authStore.user?.subtitle_prefs
    ? { ...authStore.user.subtitle_prefs }
    : { ...DEFAULT_SUBTITLE_PREFS },
)

const subtitlePreviewStyle = computed(() => ({
  color:           localPrefs.value.color,
  backgroundColor: localPrefs.value.bg_opacity > 0 ? `rgba(0,0,0,${localPrefs.value.bg_opacity})` : 'transparent',
  fontSize:        `${localPrefs.value.font_size}%`,
  textShadow:      SHADOW_MAP[localPrefs.value.text_style] ?? 'none',
  fontFamily:      localPrefs.value.font_family,
}))

const subtitleLoading = ref(false)

async function saveSubtitlePrefs() {
  subtitleLoading.value = true
  try {
    const updated = await authApi.updateSubtitlePrefs(localPrefs.value)
    if (authStore.user) authStore.user.subtitle_prefs = updated
    showToast('Subtitle settings saved', 'success')
  } catch {
    showToast('Failed to save subtitle settings', 'error')
  } finally {
    subtitleLoading.value = false
  }
}

// --- Display name ---
const displayName = ref(authStore.user?.display_name ?? '')
const displayNameLoading = ref(false)
const displayNameError = ref('')
const displayNameSuccess = ref(false)

async function saveDisplayName() {
  displayNameLoading.value = true
  displayNameError.value = ''
  displayNameSuccess.value = false
  try {
    const updated = await authApi.updateProfile(displayName.value)
    authStore.user = updated
    displayNameSuccess.value = true
  } catch (e: unknown) {
    displayNameError.value = e instanceof Error ? e.message : 'Failed to save'
  } finally {
    displayNameLoading.value = false
  }
}

// --- Change password ---
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
              class="size-10 rounded-full bg-blue-600 flex items-center justify-center text-sm font-semibold text-white uppercase shrink-0"
            >
              {{ (authStore.user?.display_name || authStore.user?.username)?.[0] }}
            </span>
            <div>
              <p class="font-medium text-white">
                {{ authStore.user?.display_name || authStore.user?.username }}
              </p>
              <p class="text-xs text-zinc-500">
                {{ authStore.user?.display_name ? authStore.user.username + ' · ' : '' }}{{ authStore.user?.is_admin ? 'Administrator' : 'User' }}
              </p>
            </div>
          </div>
          <div class="pt-2 border-t border-zinc-800">
            <p class="text-xs text-zinc-500 font-medium mb-0.5">Root directory</p>
            <p class="text-sm text-zinc-300 font-mono">{{ authStore.user?.root_dir || '—' }}</p>
          </div>
        </div>
      </div>

      <AppAccordion>

        <!-- Subtitle appearance -->
        <AppAccordionItem title="Subtitle appearance">
          <div class="space-y-4 pt-2">
            <div class="rounded-lg flex flex-col items-center justify-end py-6 gap-1 select-none" style="background: linear-gradient(160deg, #1e3040 0%, #2a3d2a 40%, #1a1a1a 100%)">
              <span :style="subtitlePreviewStyle" class="rounded px-1.5 py-0.5 text-sm transition-all duration-150">
                I should have stayed in bed today.
              </span>
              <span :style="subtitlePreviewStyle" class="rounded px-1.5 py-0.5 text-sm transition-all duration-150">
                Yeah, that would've been smarter.
              </span>
            </div>

            <div class="space-y-2.5">
              <div v-for="row in [
                { label: 'Color',      field: 'color',       options: colorOptions },
                { label: 'Background', field: 'bg_opacity',  options: bgOptions },
                { label: 'Style',      field: 'text_style',  options: styleOptions },
                { label: 'Size',       field: 'font_size',   options: sizeOptions },
                { label: 'Font',       field: 'font_family', options: fontOptions },
              ]" :key="row.field" class="flex items-center gap-3">
                <label class="text-xs font-medium text-zinc-400 w-24 shrink-0">{{ row.label }}</label>
                <AppSelect
                  :model-value="(localPrefs as any)[row.field]"
                  :options="row.options"
                  @update:model-value="(localPrefs as any)[row.field] = $event"
                />
              </div>
            </div>

            <div class="flex justify-end">
              <button
                :disabled="subtitleLoading"
                class="flex items-center gap-1.5 text-sm bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed text-white px-4 py-2 rounded-lg transition-colors"
                @click="saveSubtitlePrefs"
              >
                <AppSpinner v-if="subtitleLoading" size="sm" />
                {{ subtitleLoading ? 'Saving…' : 'Save' }}
              </button>
            </div>
          </div>
        </AppAccordionItem>

        <!-- Display name -->
        <AppAccordionItem title="Display name">
          <form class="space-y-3 pt-2" @submit.prevent="saveDisplayName">
            <div
              v-if="displayNameSuccess"
              class="text-sm text-green-400 bg-green-950/50 border border-green-900 rounded-lg px-3 py-2"
            >
              Display name updated.
            </div>
            <div
              v-if="displayNameError"
              class="text-sm text-red-400 bg-red-950/50 border border-red-900 rounded-lg px-3 py-2"
            >
              {{ displayNameError }}
            </div>
            <div>
              <label class="block text-xs font-medium text-zinc-400 mb-1.5">Name shown in the header</label>
              <input
                v-model="displayName"
                type="text"
                placeholder="Your name"
                class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-white placeholder-zinc-500 focus:outline-none focus:border-blue-500"
              />
            </div>
            <div class="flex justify-end">
              <button
                type="submit"
                :disabled="displayNameLoading"
                class="flex items-center gap-1.5 text-sm bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed text-white px-4 py-2 rounded-lg transition-colors"
              >
                <AppSpinner v-if="displayNameLoading" size="sm" />
                {{ displayNameLoading ? 'Saving…' : 'Save' }}
              </button>
            </div>
          </form>
        </AppAccordionItem>

        <!-- Change password -->
        <AppAccordionItem title="Change password">
          <form class="space-y-3 pt-2" @submit.prevent="changePassword">
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
            <div class="flex justify-end">
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
        </AppAccordionItem>

      </AppAccordion>
    </div>
  </div>
</template>
