<script setup lang="ts">
import { computed } from 'vue'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'
import AppModal from '@/components/ui/AppModal.vue'

const props = defineProps<{ entry: FileEntry | null }>()
const emit = defineEmits<{ close: [] }>()

const streamUrl = computed(() =>
  props.entry ? filesApi.streamUrl(props.entry.path) : '',
)

const mime = computed(() => props.entry?.mime ?? '')

const isImage = computed(() => mime.value.startsWith('image/'))
const isVideo = computed(() => mime.value.startsWith('video/'))
const isAudio = computed(() => mime.value.startsWith('audio/'))
const isPdf = computed(() => mime.value === 'application/pdf')
</script>

<template>
  <AppModal
    v-if="entry"
    :title="entry.name"
    size="xl"
    @close="emit('close')"
  >
    <div class="flex items-center justify-center min-h-48">
      <!-- Image -->
      <img
        v-if="isImage"
        :src="streamUrl"
        :alt="entry.name"
        class="max-w-full max-h-[70vh] object-contain rounded-lg"
      />

      <!-- Video -->
      <video
        v-else-if="isVideo"
        :src="streamUrl"
        controls
        class="max-w-full max-h-[70vh] rounded-lg"
        preload="metadata"
      />

      <!-- Audio -->
      <div v-else-if="isAudio" class="w-full px-4">
        <div class="flex items-center gap-4 mb-4">
          <svg class="size-16 text-pink-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="1.5"
              d="M9 19V6l12-3v13M9 19c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zm12-3c0 1.105-1.343 2-3 2s-3-.895-3-2 1.343-2 3-2 3 .895 3 2zM9 10l12-3"
            />
          </svg>
          <div>
            <p class="font-medium text-white">{{ entry.name }}</p>
          </div>
        </div>
        <audio :src="streamUrl" controls class="w-full" />
      </div>

      <!-- PDF -->
      <iframe
        v-else-if="isPdf"
        :src="streamUrl"
        class="w-full h-[70vh] rounded-lg border border-zinc-700"
        title="PDF preview"
      />
    </div>

    <template #footer>
      <div class="flex justify-end">
        <a
          :href="filesApi.downloadUrl(entry.path)"
          target="_blank"
          class="flex items-center gap-1.5 text-sm bg-zinc-800 hover:bg-zinc-700 text-white px-4 py-2 rounded-lg transition-colors"
        >
          <svg class="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"
            />
          </svg>
          Download
        </a>
      </div>
    </template>
  </AppModal>
</template>
