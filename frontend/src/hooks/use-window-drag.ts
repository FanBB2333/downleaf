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

  const onMouseDown = useCallback((e: React.MouseEvent) => {
    // Only respond to left button
    if (e.button !== 0) return

    // Don't drag if user clicked an interactive element
    const target = e.target as HTMLElement
    if (target.closest('button, a, input, select, textarea, [role="button"], [data-no-drag]')) {
      return
    }

    e.preventDefault()
    dragging.current = true

    const startScreenX = e.screenX
    const startScreenY = e.screenY

    let startWinX = 0
    let startWinY = 0

    WindowGetPosition().then((pos) => {
      startWinX = pos.x
      startWinY = pos.y
    })

    const onMouseMove = (ev: MouseEvent) => {
      if (!dragging.current) return
      const dx = ev.screenX - startScreenX
      const dy = ev.screenY - startScreenY
      WindowSetPosition(startWinX + dx, startWinY + dy)
    }

    const onMouseUp = () => {
      dragging.current = false
      document.removeEventListener('mousemove', onMouseMove)
      document.removeEventListener('mouseup', onMouseUp)
    }

    document.addEventListener('mousemove', onMouseMove)
    document.addEventListener('mouseup', onMouseUp)
  }, [])

  return onMouseDown
}
