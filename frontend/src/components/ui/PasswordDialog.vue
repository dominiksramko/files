<script setup lang="ts">
import { nextTick, ref, watch, type ComponentPublicInstance } from 'vue'
import type { PasswordPrompt } from '@/composables/usePasswordPrompt'
import AppButton from '@/components/ui/AppButton.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import SecretInput from '@/components/ui/SecretInput.vue'

// The dialog of a usePasswordPrompt(); action names what the password is for
// ("Extract"), busyLabel what's shown while it's tried.
const props = defineProps<{ prompt: PasswordPrompt; action: string; busyLabel: string }>()

const password = ref('')
const input = ref<ComponentPublicInstance | null>(null)
watch(() => props.prompt.visible.value, (visible) => {
  if (!visible) return
  password.value = ''
  nextTick(() => (input.value?.$el as HTMLInputElement | undefined)?.focus())
})
</script>

<template>
  <AppDialog :open="prompt.visible.value" @close="prompt.cancel()">
    <h3 class="text-sm font-semibold text-zinc-100 mb-1">Password required</h3>
    <p class="text-xs text-zinc-400 mb-3">This archive is password protected.</p>
    <SecretInput
      v-model="password"
      ref="input"
      placeholder="Enter password"
      class="w-full bg-zinc-800 text-zinc-100 placeholder-zinc-500 rounded-lg px-3 py-2 text-sm border border-transparent focus:outline-none focus:border-zinc-600 transition-colors"
      @keydown.enter="prompt.submit(password)"
    />
    <p v-if="prompt.wrong.value" class="text-xs text-red-400 mt-2">Wrong password</p>
    <div class="flex gap-2 mt-3 justify-end">
      <AppButton :disabled="prompt.busy.value" @click="prompt.cancel()">Cancel</AppButton>
      <AppButton variant="primary" :disabled="prompt.busy.value || !password" @click="prompt.submit(password)">
        {{ prompt.busy.value ? busyLabel : action }}
      </AppButton>
    </div>
  </AppDialog>
</template>
