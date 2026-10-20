import { onUnmounted, watch, type Ref } from 'vue'
import { topModal } from '@/utils/keyboard'

// Closes the dialog card while it's open, when Escape is pressed and it's the
// dialog on top (another can open over it). A key an input already handled
// (preventDefault, e.g. cancelling an edit) is left alone.
export function useEscapeToClose(card: Readonly<Ref<HTMLElement | null>>, open: () => boolean, close: () => void) {
  function onKeydown(event: KeyboardEvent) {
    if (event.key !== 'Escape' || event.defaultPrevented) return
    if (!card.value || topModal() !== card.value) return
    event.preventDefault()
    close()
  }

  watch(open, (isOpen) => {
    if (isOpen) window.addEventListener('keydown', onKeydown)
    else window.removeEventListener('keydown', onKeydown)
  }, { immediate: true })
  onUnmounted(() => window.removeEventListener('keydown', onKeydown))
}
