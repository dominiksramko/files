<script setup lang="ts">
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import AppIcon from '@/components/ui/AppIcon.vue'

const props = defineProps<{ entry: FileEntry }>()

const streamUrl = filesApi.streamUrl(props.entry.path, props.entry.mod_time)
</script>

<template>
  <div class="w-full max-w-md px-6" @click.stop>
    <div class="flex items-center gap-5 mb-6">
      <div class="size-20 rounded-2xl bg-pink-950/60 flex items-center justify-center shrink-0">
        <AppIcon name="music" class="size-10 text-pink-400" :stroke-width="1.5" />
      </div>
      <p class="text-white font-medium text-lg leading-tight">{{ entry.name }}</p>
    </div>
    <audio :src="streamUrl" controls class="w-full" />
  </div>
</template>
