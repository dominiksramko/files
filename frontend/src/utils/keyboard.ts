// Whether a key press is part of typing — in an input, textarea, select or
// rich-text field — rather than a page-wide shortcut.
export function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName) || target.isContentEditable === true
}

// Whether a dialog is open: it owns the keyboard, so page-wide and preview
// shortcuts (Escape, arrows) leave the page behind it alone.
export function isModalOpen(): boolean {
  return document.querySelector('[aria-modal="true"]') !== null
}
