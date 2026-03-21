/**
 * Returns a debounced version of `fn` that delays invoking it until `delay` ms
 * have elapsed since the last call. Call `.cancel()` to clear any pending timer.
 */
export function debounce<T extends (...args: never[]) => void>(fn: T, delay: number): T & { cancel: () => void } {
  let timer: ReturnType<typeof setTimeout> | null = null

  const debounced = (...args: Parameters<T>) => {
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => { fn(...args) }, delay)
  }

  debounced.cancel = () => {
    if (timer) { clearTimeout(timer); timer = null }
  }

  return debounced as T & { cancel: () => void }
}
