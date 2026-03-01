<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import Plyr from 'plyr'
import 'plyr/dist/plyr.css'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'

const props = defineProps<{ entry: FileEntry }>()

// Derived once at mount; parent remounts this component (via :key) on path change.
const needsRemux = ['video/x-matroska', 'video/x-msvideo', 'video/quicktime'].includes(props.entry.mime)
const src        = needsRemux ? undefined : filesApi.streamUrl(props.entry.path, props.entry.mod_time)
const sourceType = needsRemux ? undefined : props.entry.mime
// Computed so it reacts if the store entry gets its thumb_hash populated by the
// SSE stream after the initial listing (thumbnail generated while folder is open).
const poster = computed(() =>
  props.entry.has_thumb && props.entry.thumb_hash
    ? filesApi.thumbUrl(props.entry.path, props.entry.thumb_hash)
    : undefined,
)

interface SubtitleTrack {
  src: string
  label: string
  srclang: string
}

const videoEl   = ref<HTMLVideoElement | null>(null)
const subtitles = ref<SubtitleTrack[]>([])
let plyrInstance: Plyr | null = null
let hlsInstance: any = null
let alive = true

const plyrOptions: Plyr.Options = {
  controls: [
    'play-large', 'play', 'rewind', 'fast-forward',
    'progress', 'current-time', 'duration',
    'mute', 'volume', 'captions', 'settings', 'pip', 'fullscreen',
  ],
  settings: ['captions', 'speed'],
  captions: { active: true, language: 'auto', update: true },
  speed: { selected: 1, options: [0.5, 0.75, 1, 1.25, 1.5, 1.75, 2] },
  keyboard: { focused: true, global: false },
}

onMounted(async () => {
  // Ask the server for all subtitle tracks (file-based + embedded).
  // This is the only network call the client makes — the server does all discovery.
  try {
    const raw = await filesApi.subtitles(props.entry.path)
    if (!alive) return
    subtitles.value = raw.map(t => ({
      src:     filesApi.subtitleUrl(t.url),
      label:   t.label,
      srclang: t.srclang,
    }))
  } catch {
    // subtitle discovery failed — continue without captions
  }

  // Wait for Vue to render <track> elements into the DOM before Plyr reads them.
  await nextTick()
  if (!alive || !videoEl.value) return

  if (needsRemux) {
    const Hls = (await import('hls.js')).default
    if (Hls.isSupported()) {
      hlsInstance = new Hls()
      hlsInstance.loadSource(filesApi.hlsPlaylistUrl(props.entry.path))
      hlsInstance.attachMedia(videoEl.value)
      
      hlsInstance.on(Hls.Events.MANIFEST_PARSED, () => {
         if (alive && videoEl.value && !plyrInstance) {
           plyrInstance = new Plyr(videoEl.value!, plyrOptions)
         }
      })
    } else if (videoEl.value.canPlayType('application/vnd.apple.mpegurl')) {
      videoEl.value.src = filesApi.hlsPlaylistUrl(props.entry.path)
      plyrInstance = new Plyr(videoEl.value, plyrOptions)
    }
  } else {
    plyrInstance = new Plyr(videoEl.value, plyrOptions)
  }
})

// Destroy Plyr before the DOM element is removed, so Plyr can clean up its
// own DOM manipulation before Vue unmounts the component.
onBeforeUnmount(() => {
  alive = false
  if (hlsInstance) {
    hlsInstance.destroy()
    hlsInstance = null
  }
  plyrInstance?.destroy()
  plyrInstance = null
})
</script>

<template>
  <div class="plyr-wrapper w-full max-w-5xl" @click.stop>
    <video ref="videoEl" preload="metadata" playsinline :poster="poster">
      <source v-if="src" :src="src" :type="sourceType" />
      <track
        v-for="(sub, i) in subtitles"
        :key="sub.src"
        kind="subtitles"
        :label="sub.label"
        :srclang="sub.srclang || undefined"
        :src="sub.src"
        :default="i === 0"
      />
    </video>
  </div>
</template>

<style scoped>
:deep(.plyr) {
  --plyr-color-main: #3b82f6;
  --plyr-video-controls-background: linear-gradient(rgba(0,0,0,0), rgba(0,0,0,0.65));
  --plyr-font-family: inherit;
  border-radius: 0.5rem;
  overflow: hidden;
  max-height: calc(100vh - 120px);
  width: 100%;
}
:deep(.plyr video) {
  max-height: calc(100vh - 120px);
  object-fit: contain;
}
</style>
