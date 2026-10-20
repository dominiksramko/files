import { defineComponent, onUnmounted, type VNodeChild } from 'vue'
import { useLayoutStore } from '@/stores/layout'

// Shows render's output in the middle of the app header (in place of the
// logo) while the calling view is mounted. render closes over the view's own
// state; AppHeader renders it without knowing what it is.
export function useHeaderCenter(name: string, render: () => VNodeChild) {
  const layoutStore = useLayoutStore()
  const component = defineComponent({ name, setup: () => render })
  layoutStore.headerBreadcrumbsActive = true
  layoutStore.headerCenter = component
  onUnmounted(() => {
    // The next view may have put its own there already.
    if (layoutStore.headerCenter !== component) return
    layoutStore.headerBreadcrumbsActive = false
    layoutStore.headerCenter = null
  })
}
