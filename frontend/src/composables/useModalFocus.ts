import { nextTick, onUnmounted, watch, type Ref } from 'vue'
import { topModal } from '@/utils/keyboard'

const focusableSelector = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[contenteditable="true"]',
  '[tabindex]:not([tabindex="-1"])',
].join(', ')

// Whether element is shown: nothing between it and container is hidden
// (v-show hides with an inline display: none).
function isShown(element: HTMLElement, container: HTMLElement): boolean {
  for (let node: HTMLElement | null = element; node && node !== container; node = node.parentElement) {
    if (node.hidden || node.style.display === 'none') return false
  }
  return true
}

// What Tab can reach inside container, in order.
export function focusableIn(container: HTMLElement): HTMLElement[] {
  return [...container.querySelectorAll<HTMLElement>(focusableSelector)].filter((element) => isShown(element, container))
}

// Keeps the keyboard in a modal card while it's open: focus moves in when it
// opens (to its [autofocus] element, else the card itself, unless something
// inside already took it), Tab and Shift+Tab go round inside it while it's
// the dialog on top, and focus goes back to what had it before when it
// closes. The card needs tabindex="-1".
export function useModalFocus(card: Readonly<Ref<HTMLElement | null>>, open: () => boolean) {
  let opener: HTMLElement | null = null
  let active = false
  // The card while it's open: a closing one can still be fading out (and
  // holding focus) after its ref is cleared.
  let openCard: HTMLElement | null = null

  function onKeydown(event: KeyboardEvent) {
    const container = card.value
    if (event.key !== 'Tab' || !container || topModal() !== container) return
    const focusable = focusableIn(container)
    const current = document.activeElement
    const inside = current instanceof Node && container.contains(current)
    if (focusable.length === 0) {
      event.preventDefault()
      container.focus()
      return
    }
    const first = focusable[0]!
    const last = focusable[focusable.length - 1]!
    if (event.shiftKey && (!inside || current === first || current === container)) {
      event.preventDefault()
      last.focus()
    } else if (!event.shiftKey && (!inside || current === last)) {
      event.preventDefault()
      first.focus()
    }
  }

  function activate() {
    if (active) return
    active = true
    const current = document.activeElement
    opener = current instanceof HTMLElement && current !== document.body ? current : null
    window.addEventListener('keydown', onKeydown)
    nextTick(() => {
      const container = card.value
      openCard = container
      if (!container || container.contains(document.activeElement)) return
      const preferred = container.querySelector<HTMLElement>('[autofocus]')
      ;(preferred ?? container).focus({ preventScroll: true })
    })
  }

  // Focus goes back only if it would otherwise be lost: still in the card, or
  // nowhere (the card is gone).
  function deactivate() {
    if (!active) return
    active = false
    window.removeEventListener('keydown', onKeydown)
    const current = document.activeElement
    const container = card.value ?? openCard
    const lost = !current || current === document.body || (container?.contains(current) ?? false)
    if (opener?.isConnected && lost) opener.focus({ preventScroll: true })
    opener = null
    openCard = null
  }

  watch(open, (isOpen) => (isOpen ? activate() : deactivate()), { immediate: true, flush: 'post' })
  onUnmounted(deactivate)
}
