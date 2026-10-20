<script setup lang="ts">
import { computed } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import { catBg, catIconColor, catIconName, fileCategory, formatSize } from '@/utils/formatters'
import AppIcon from '@/components/ui/AppIcon.vue'

const props = defineProps<{ entry: FileEntry }>()

const category = computed(() => fileCategory(props.entry.mime, props.entry.name))
</script>

<template>
  <div class="flex flex-col items-center gap-6 p-8 w-full" @click.stop>
    <div :class="['size-24 rounded-2xl flex items-center justify-center', catBg[category]]">
      <AppIcon :name="catIconName[category]" :class="['size-12', catIconColor[category]]" :stroke-width="1.5" />
    </div>
    <div class="text-center space-y-1">
      <p class="text-white font-semibold text-lg leading-tight break-all">{{ entry.name }}</p>
      <p class="text-zinc-400 text-sm">{{ formatSize(entry.size) }}</p>
      <p class="text-zinc-600 text-xs font-mono">{{ entry.mime || 'Unknown type' }}</p>
    </div>
    <a
      :href="filesApi.downloadUrl(entry.path, entry.mod_time)"
      target="_blank"
      class="flex items-center gap-2 bg-blue-600 hover:bg-blue-700 text-white font-medium text-sm px-6 py-2.5 rounded-xl transition-colors"
      @click.stop
    >
      <AppIcon name="download" class="size-4" />
      Download
    </a>
  </div>
</template>
