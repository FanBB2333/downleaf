import { useCallback, useRef } from 'react'
import { WindowGetPosition, WindowSetPosition } from '../../wailsjs/runtime/runtime'

/**
 * Hook that returns an onMouseDown handler for window dragging.
 * Attach it to any element that should act as a drag handle.
 * Interactive children should call e.stopPropagation() on mousedown
 * to prevent triggering the drag.
 */
export function useWindowDrag() {
  const dragging = useRef(false)
  const rafId = useRef(0)

  const onMouseDown = useCallback((e: React.MouseEvent) => {
    if (e.button !== 0) return

    const target = e.target as HTMLElement
    if (target.closest('button, a, input, select, textarea, [role="button"], [data-no-drag]')) {
      return
    }

    e.preventDefault()
    dragging.current = true

    const startScreenX = e.screenX
    const startScreenY = e.screenY

    // Get the current window position before attaching move listeners
    WindowGetPosition().then((pos) => {
      if (!dragging.current) return

      const startWinX = pos.x
      const startWinY = pos.y

      const onMouseMove = (ev: MouseEvent) => {
        if (!dragging.current) return
        cancelAnimationFrame(rafId.current)
        rafId.current = requestAnimationFrame(() => {
          const dx = ev.screenX - startScreenX
          const dy = ev.screenY - startScreenY
          WindowSetPosition(startWinX + dx, startWinY + dy)
        })
      }

      const onMouseUp = () => {
        dragging.current = false
        cancelAnimationFrame(rafId.current)
        document.removeEventListener('mousemove', onMouseMove)
        document.removeEventListener('mouseup', onMouseUp)
      }

      document.addEventListener('mousemove', onMouseMove)
      document.addEventListener('mouseup', onMouseUp)
    })
  }, [])

  return onMouseDown
}
