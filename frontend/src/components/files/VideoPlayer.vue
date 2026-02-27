<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import Plyr from 'plyr'
import 'plyr/dist/plyr.css'
import type { FileEntry } from '@/types'
import { files as filesApi } from '@/utils/api'

const props = defineProps<{ entry: FileEntry }>()

// Derived once at mount; parent remounts this component (via :key) on path change.
const src    = filesApi.streamUrl(props.entry.path)
const poster = props.entry.has_thumb ? filesApi.thumbUrl(props.entry.path) : undefined

// Formats that the server remuxes to fragmented MP4 on the fly.
// The <source type> must reflect what the server actually sends so browsers
// don't reject the request before it even reaches the network.
function effectiveMime(mime: string): string {
  switch (mime) {
    case 'video/x-matroska':
    case 'video/x-msvideo':
    case 'video/quicktime':
      return 'video/mp4'
    default:
      return mime
  }
}
const sourceType = effectiveMime(props.entry.mime)

interface SubtitleTrack {
  src: string
  label: string
  srclang: string
}

const videoEl   = ref<HTMLVideoElement | null>(null)
const subtitles = ref<SubtitleTrack[]>([])
let plyrInstance: Plyr | null = null
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

  plyrInstance = new Plyr(videoEl.value, plyrOptions)
})

// Destroy Plyr before the DOM element is removed, so Plyr can clean up its
// own DOM manipulation before Vue unmounts the component.
onBeforeUnmount(() => {
  alive = false
  plyrInstance?.destroy()
  plyrInstance = null
})
</script>

<template>
  <div class="plyr-wrapper w-full max-w-5xl" @click.stop>
    <video ref="videoEl" preload="metadata" playsinline :poster="poster">
      <source :src="src" :type="sourceType" />
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
