<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi, NotFoundError } from '@/utils/api'
import { compareFolderItems } from '@/utils/archiveTree'
import { formatSize } from '@/utils/formatters'
import AppButton from '@/components/ui/AppButton.vue'
import AppCheckbox from '@/components/ui/AppCheckbox.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppLoading from '@/components/ui/AppLoading.vue'
import AppModal from '@/components/ui/AppModal.vue'
import FileBreadcrumb from '@/components/files/FileBreadcrumb.vue'
import FileIcon from '@/components/files/FileIcon.vue'

// Chooses files from the user's folder, starting in folder. Files can be
// ticked in several folders before choosing; double-clicking a file chooses
// just that one. A form for an action, so it isn't kept in the URL.
const props = withDefaults(defineProps<{ title: string; folder: string; chooseLabel?: string }>(), { chooseLabel: 'Choose' })

const emit = defineEmits<{
  close: []
  choose: [paths: string[]]
}>()

const shownFolder = ref(props.folder)
const entries = ref<FileEntry[]>([])
const loading = ref(false)
const loadError = ref('')
const selected = ref<string[]>([])

// Hidden files left out, folders first.
const shownEntries = computed(() => entries.value.filter((entry) => !entry.name.startsWith('.')).sort(compareFolderItems))

let loadingFolder = ''
async function load(folder: string) {
  loadingFolder = folder
  loading.value = true
  loadError.value = ''
  try {
    const listed = await filesApi.list(folder)
    if (loadingFolder === folder) entries.value = listed
  } catch (err) {
    if (loadingFolder !== folder) return
    // A folder that's gone: show the nearest one still there.
    if (err instanceof NotFoundError && err.fallbackPath !== folder) {
      shownFolder.value = err.fallbackPath
      return
    }
    entries.value = []
    loadError.value = (err as Error).message
  } finally {
    if (loadingFolder === folder) loading.value = false
  }
}

watch(shownFolder, load, { immediate: true })

function toggle(path: string) {
  selected.value = selected.value.includes(path)
    ? selected.value.filter((chosen) => chosen !== path)
    : [...selected.value, path]
}
</script>

<template>
  <AppModal :title="title" size="md" fixed-height @close="emit('close')">
    <div class="flex h-full flex-col gap-3">
      <div class="flex h-10 shrink-0 items-center rounded-lg border border-zinc-800 bg-zinc-950/60 px-1">
        <FileBreadcrumb :path="shownFolder" @navigate="shownFolder = $event" />
      </div>

      <AppLoading v-if="loading" label="Reading the folder…" />
      <p v-else-if="loadError" class="py-12 text-center text-sm text-red-300">{{ loadError }}</p>
      <p v-else-if="!shownEntries.length" class="py-12 text-center text-sm text-zinc-500">This folder is empty</p>
      <ul v-else class="-mx-2 space-y-0.5" data-picker-entries>
        <li v-for="entry in shownEntries" :key="entry.path">
          <button
            v-if="entry.is_dir"
            type="button"
            class="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left text-sm text-zinc-200 transition-colors hover:bg-zinc-800"
            @click="shownFolder = entry.path"
          >
            <FileIcon :mime="entry.mime" :name="entry.name" :is-dir="true" />
            <span class="min-w-0 flex-1 truncate">{{ entry.name }}</span>
            <AppIcon name="chevronRight" class="size-4 text-zinc-500" />
          </button>
          <div
            v-else
            role="button"
            tabindex="0"
            class="flex cursor-pointer select-none items-center gap-3 rounded-lg px-2 py-2 text-sm text-zinc-200 transition-colors hover:bg-zinc-800"
            :class="{ 'bg-blue-500/10': selected.includes(entry.path) }"
            @click="toggle(entry.path)"
            @keydown.enter.prevent="toggle(entry.path)"
            @keydown.space.prevent="toggle(entry.path)"
            @dblclick="emit('choose', [entry.path])"
          >
            <AppCheckbox :model-value="selected.includes(entry.path)" tabindex="-1" @update:model-value="toggle(entry.path)" />
            <FileIcon :mime="entry.mime" :name="entry.name" :is-dir="false" />
            <span class="min-w-0 flex-1 truncate">{{ entry.name }}</span>
            <span class="shrink-0 whitespace-nowrap text-xs tabular-nums text-zinc-500">{{ formatSize(entry.size) }}</span>
          </div>
        </li>
      </ul>
    </div>

    <template #footer>
      <div class="flex items-center gap-2">
        <span class="text-xs text-zinc-500">
          {{ selected.length ? `${selected.length} file${selected.length === 1 ? '' : 's'} selected` : 'Tick the files to choose' }}
        </span>
        <div class="ml-auto flex items-center gap-2">
          <AppButton @click="emit('close')">Cancel</AppButton>
          <AppButton variant="primary" :disabled="!selected.length" @click="emit('choose', selected)">{{ chooseLabel }}</AppButton>
        </div>
      </div>
    </template>
  </AppModal>
</template>
