<script setup lang="ts">
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'

const props = defineProps<{ entry: FileEntry }>()

const streamUrl = filesApi.streamUrl(props.entry.path)
</script>

<template>
  <div class="w-full" @click.stop>
    <iframe
      :src="streamUrl"
      class="w-full rounded-lg border border-zinc-700"
      style="height: calc(100vh - 120px)"
      title="PDF preview"
    />
  </div>
</template>
