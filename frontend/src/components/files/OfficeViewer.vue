<script setup lang="ts">
import { ref, onMounted } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import AppSpinner from '@/components/ui/AppSpinner.vue'

const props = defineProps<{ entry: FileEntry }>()

const iframeSrc = ref('')
const error = ref('')

onMounted(async () => {
  try {
    const { token } = await filesApi.officeToken(props.entry.path)
    const fileUrl = `${window.location.origin}/public/office/${token}`
    iframeSrc.value = `https://view.officeapps.live.com/op/embed.aspx?src=${encodeURIComponent(fileUrl)}`
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Failed to load preview'
  }
})
</script>

<template>
  <div class="w-full" @click.stop>
    <div v-if="error" class="flex items-center justify-center h-full text-sm text-zinc-400">
      {{ error }}
    </div>
    <div v-else-if="!iframeSrc" class="flex items-center justify-center" style="height: calc(100vh - 120px)">
      <AppSpinner />
    </div>
    <iframe
      v-else
      :src="iframeSrc"
      class="w-full rounded-lg border border-zinc-700"
      style="height: calc(100vh - 120px)"
      title="Office preview"
    />
  </div>
</template>
