export type WheelZoomModifier = 'ctrl' | 'shift' | 'alt'

export const wheelZoomLabels: Record<WheelZoomModifier, string> = { ctrl: 'Ctrl', shift: 'Shift', alt: 'Alt' }

export function isWheelZoomModifier(value: string | null): value is WheelZoomModifier {
  return value === 'ctrl' || value === 'shift' || value === 'alt'
}

export function matchesWheelZoom(event: Pick<WheelEvent, 'ctrlKey' | 'shiftKey' | 'altKey'>, modifier: WheelZoomModifier) {
  return modifier === 'ctrl' ? event.ctrlKey : modifier === 'shift' ? event.shiftKey : event.altKey
}
