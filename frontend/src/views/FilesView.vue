<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useFilesStore } from '@/stores/files'
import type { FileEntry } from '@/types'
import FileBreadcrumb from '@/components/files/FileBreadcrumb.vue'
import FileList from '@/components/files/FileList.vue'
import FileGrid from '@/components/files/FileGrid.vue'
import FilePreview from '@/components/files/FilePreview.vue'
import ArchiveContents from '@/components/archive/ArchiveContents.vue'

const filesStore = useFilesStore()

const previewEntry = ref<FileEntry | null>(null)
const archiveEntry = ref<FileEntry | null>(null)
const viewMode = ref<'grid' | 'list'>(
  (localStorage.getItem('viewMode') as 'grid' | 'list') ?? 'grid',
)

function setView(mode: 'grid' | 'list') {
  viewMode.value = mode
  localStorage.setItem('viewMode', mode)
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
    <div class="px-4 py-2.5 border-b border-zinc-800 flex items-center gap-3 bg-zinc-900 shrink-0">
      <FileBreadcrumb :path="filesStore.currentPath" @navigate="filesStore.navigate" />
      <div class="flex-1" />

      <!-- View toggle -->
      <div class="flex items-center bg-zinc-800 rounded-lg p-0.5 gap-0.5">
        <button
          :class="[
            'p-1.5 rounded-md transition-colors',
            viewMode === 'grid' ? 'bg-zinc-600 text-white' : 'text-zinc-400 hover:text-white',
          ]"
          title="Grid view"
          @click="setView('grid')"
        >
          <svg class="size-4" fill="currentColor" viewBox="0 0 24 24">
            <path d="M4 4h6v6H4V4zm0 10h6v6H4v-6zm10-10h6v6h-6V4zm0 10h6v6h-6v-6z" />
          </svg>
        </button>
        <button
          :class="[
            'p-1.5 rounded-md transition-colors',
            viewMode === 'list' ? 'bg-zinc-600 text-white' : 'text-zinc-400 hover:text-white',
          ]"
          title="List view"
          @click="setView('list')"
        >
          <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M4 6h16M4 10h16M4 14h16M4 18h16"
            />
          </svg>
        </button>
      </div>

      <!-- Refresh -->
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

    <!-- Content -->
    <div class="flex-1 overflow-hidden">
      <FileGrid
        v-if="viewMode === 'grid'"
        :entries="filesStore.entries"
        :loading="filesStore.loading"
        @navigate="filesStore.navigate"
        @preview="previewEntry = $event"
        @archive-preview="archiveEntry = $event"
      />
      <FileList
        v-else
        :entries="filesStore.entries"
        :loading="filesStore.loading"
        :current-path="filesStore.currentPath"
        @navigate="filesStore.navigate"
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
