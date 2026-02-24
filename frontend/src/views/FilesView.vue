<script setup lang="ts">
import { onMounted } from 'vue'
import { useFilesStore } from '@/stores/files'
import type { FileEntry } from '@/types'
import { ref } from 'vue'
import FileBreadcrumb from '@/components/files/FileBreadcrumb.vue'
import FileList from '@/components/files/FileList.vue'
import FilePreview from '@/components/files/FilePreview.vue'
import ArchiveContents from '@/components/archive/ArchiveContents.vue'

const filesStore = useFilesStore()

const previewEntry = ref<FileEntry | null>(null)
const archiveEntry = ref<FileEntry | null>(null)

function navigate(path: string) {
  filesStore.navigate(path)
}

function onExtracted() {
  archiveEntry.value = null
  filesStore.refresh()
}

onMounted(() => filesStore.navigate(''))
</script>

<template>
  <div class="h-full flex flex-col">
    <!-- Toolbar -->
    <div class="px-4 py-3 border-b border-zinc-800 flex items-center gap-3 bg-zinc-900 shrink-0">
      <FileBreadcrumb :path="filesStore.currentPath" @navigate="navigate" />
      <div class="flex-1" />
      <button
        class="text-zinc-400 hover:text-white transition-colors p-1.5 rounded hover:bg-zinc-800"
        title="Refresh"
        @click="filesStore.refresh()"
      >
        <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
          />
        </svg>
      </button>
    </div>

    <!-- Error banner -->
    <div
      v-if="filesStore.error"
      class="px-4 py-2 text-sm text-red-400 bg-red-950/30 border-b border-red-900 shrink-0"
    >
      {{ filesStore.error }}
    </div>

    <!-- File list -->
    <div class="flex-1 overflow-hidden">
      <FileList
        :entries="filesStore.entries"
        :loading="filesStore.loading"
        :current-path="filesStore.currentPath"
        @navigate="navigate"
        @preview="previewEntry = $event"
        @archive-preview="archiveEntry = $event"
      />
    </div>

    <!-- Modals -->
    <FilePreview :entry="previewEntry" @close="previewEntry = null" />
    <ArchiveContents
      v-if="archiveEntry"
      :entry="archiveEntry"
      @close="archiveEntry = null"
      @extracted="onExtracted"
    />
  </div>
</template>
