import { watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import type { SubtitlePrefs } from '@/types'

let styleEl: HTMLStyleElement | null = null

const SHADOW_MAP: Record<string, string> = {
  'none':        'none',
  'raised':      'rgb(0,0,0) 4px 4px 0px',
  'depressed':   'rgb(0,0,0) 4px 4px 0px, rgba(0,0,0,0.25) 0px 4px 4px',
  'uniform':     'rgb(0,0,0) -3px 0px, rgb(0,0,0) 0px 3px, rgb(0,0,0) 3px 0px, rgb(0,0,0) 0px -3px',
  'drop-shadow': 'rgba(0,0,0,0.75) 4px 4px 4px',
}

function buildCss(p: SubtitlePrefs): string {
  const bg     = p.bg_opacity > 0 ? `rgba(0,0,0,${p.bg_opacity})` : 'transparent'
  const shadow = SHADOW_MAP[p.text_style] ?? 'none'
  const font   = p.font_family || 'sans-serif'
  // ::cue targets native browser track rendering
  // .plyr__caption targets Plyr's own JS-rendered captions
  return (
    `::cue{color:${p.color};background-color:${bg};font-size:${p.font_size}%;text-shadow:${shadow};font-family:${font}}` +
    `.plyr__caption{color:${p.color}!important;background-color:${bg}!important;font-size:${p.font_size}%!important;text-shadow:${shadow}!important;font-family:${font}!important}`
  )
}

export function useSubtitleStyle() {
  const auth = useAuthStore()

  watch(
    () => auth.user?.subtitle_prefs,
    (prefs) => {
      if (!prefs) return
      if (!styleEl) {
        styleEl = document.createElement('style')
        styleEl.id = 'cue-style'
        document.head.appendChild(styleEl)
      }
      styleEl.textContent = buildCss(prefs)
    },
    { immediate: true },
  )
}
