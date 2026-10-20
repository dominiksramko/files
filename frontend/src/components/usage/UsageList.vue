<script setup lang="ts">
import type { UsageEntry } from '@/types'
import { canOpen, formatShare, shareOf } from '@/utils/diskUsage'
import { formatDate, formatSize } from '@/utils/formatters'
import { parentOf, relativeTo } from '@/utils/paths'
import AppCheckbox from '@/components/ui/AppCheckbox.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import FileIcon from '@/components/files/FileIcon.vue'

// Items with a bar for the part of total each takes. With showPaths, each
// shows where it is inside root (the largest files of a whole scan).
const props = defineProps<{
  entries: UsageEntry[]
  total: number
  showPaths?: boolean
  root: string
}>()
const selected = defineModel<string[]>('selected', { required: true })
const emit = defineEmits<{ open: [entry: UsageEntry]; reveal: [entry: UsageEntry]; delete: [entry: UsageEntry] }>()

function toggle(entry: UsageEntry) {
  selected.value = selected.value.includes(entry.path)
    ? selected.value.filter((path) => path !== entry.path)
    : [...selected.value, entry.path]
}

function onRowClick(entry: UsageEntry) {
  if (canOpen(entry)) emit('open', entry)
  else toggle(entry)
}

function share(entry: UsageEntry): number {
  return shareOf(entry.disk, props.total)
}

function itemCount(entry: UsageEntry): string {
  if (!entry.is_dir || entry.link) return ''
  if (!entry.scanned) return 'not scanned'
  const count = entry.files + entry.dirs
  return `${count.toLocaleString()} item${count !== 1 ? 's' : ''}`
}

function folderOf(entry: UsageEntry): string {
  return relativeTo(props.root, parentOf(entry.path))
}
</script>

<template>
  <ul class="divide-y divide-zinc-800/60">
    <li
      v-for="entry in entries"
      :key="entry.path"
      :class="[
        'group flex items-center gap-2 sm:gap-3 px-2 sm:px-3 py-2 cursor-pointer transition-colors',
        selected.includes(entry.path) ? 'bg-blue-950/40' : 'hover:bg-zinc-800/50',
      ]"
      @click="onRowClick(entry)"
    >
      <AppCheckbox :model-value="selected.includes(entry.path)" @update:model-value="toggle(entry)" />
      <FileIcon :mime="entry.mime" :name="entry.name" :is-dir="entry.is_dir" />
      <div class="min-w-0 flex-1">
        <div class="flex items-baseline gap-2 min-w-0">
          <span class="truncate text-sm text-zinc-100">
            <span v-if="showPaths && folderOf(entry)" class="text-zinc-500">{{ folderOf(entry) }}/</span>{{ entry.name }}
          </span>
          <span v-if="entry.link" class="shrink-0 text-[10px] uppercase tracking-wide text-zinc-500">link</span>
          <span
            v-if="entry.hard_links"
            class="shrink-0 text-[10px] uppercase tracking-wide text-zinc-500"
            :title="`Shares its space with ${entry.hard_links - 1} other name${entry.hard_links > 2 ? 's' : ''}`"
          >hard link</span>
        </div>
        <div class="mt-1 h-1.5 rounded-full bg-zinc-800 overflow-hidden">
          <div class="h-full rounded-full bg-blue-500/70" :style="{ width: `${share(entry) * 100}%` }" />
        </div>
      </div>
      <span class="w-16 sm:w-20 shrink-0 text-right text-sm text-zinc-200 tabular-nums">{{ formatSize(entry.disk) }}</span>
      <span class="hidden sm:block w-9 shrink-0 text-right text-xs text-zinc-500 tabular-nums">{{ formatShare(share(entry)) }}</span>
      <span class="hidden md:block w-24 shrink-0 text-right text-xs text-zinc-500 tabular-nums">{{ itemCount(entry) }}</span>
      <span class="hidden lg:block w-36 shrink-0 text-right text-xs text-zinc-500">{{ formatDate(entry.mod_time) }}</span>
      <div class="flex shrink-0 sm:opacity-0 sm:group-hover:opacity-100 focus-within:opacity-100 transition-opacity">
        <button
          type="button"
          class="p-1.5 rounded text-zinc-400 hover:text-white hover:bg-zinc-700"
          title="Show in Files"
          aria-label="Show in Files"
          @click.stop="emit('reveal', entry)"
        >
          <AppIcon name="folderOutline" class="size-4" />
        </button>
        <button
          type="button"
          class="p-1.5 rounded text-zinc-400 hover:text-red-400 hover:bg-zinc-700"
          title="Delete"
          aria-label="Delete"
          @click.stop="emit('delete', entry)"
        >
          <AppIcon name="trash" class="size-4" />
        </button>
      </div>
    </li>
  </ul>
</template>
