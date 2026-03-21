import { defineStore } from 'pinia'
import { ref, shallowRef, type Component } from 'vue'

export const useLayoutStore = defineStore('layout', () => {
  // True while a view has registered a header center component
  const headerBreadcrumbsActive = ref(false)
  // The component to render in the header center slot — set by individual views
  const headerCenter = shallowRef<Component | null>(null)

  return { headerBreadcrumbsActive, headerCenter }
})
