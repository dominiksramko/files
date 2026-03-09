<script setup lang="ts">
import { computed } from 'vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import { entryCategory, catIconColor, catIconName } from '@/utils/formatters'

const props = defineProps<{ mime: string; name: string; isDir: boolean }>()

const cat = computed(() => entryCategory(props.mime, props.name, props.isDir))
</script>

<template>
  <AppIcon
    :name="catIconName[cat]"
    :class="['size-5 shrink-0', catIconColor[cat]]"
    :stroke-width="1.5"
  />
</template>
