/**
 * Return focus after a pointer-driven close without painting the keyboard
 * focus ring. The ring comes back as soon as the element sees a key press,
 * and the marker is dropped when focus leaves.
 */
export function focusQuietly(element: HTMLElement | null | undefined) {
  if (!element) return
  element.dataset.quietFocus = ''
  const clear = () => {
    delete element.dataset.quietFocus
    element.removeEventListener('blur', clear)
    element.removeEventListener('keydown', clear)
  }
  element.addEventListener('blur', clear)
  element.addEventListener('keydown', clear)
  element.focus({ preventScroll: true })
}
