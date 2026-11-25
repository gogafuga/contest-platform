export type EventType = 'copy' | 'paste' | 'tab_hidden' | 'tab_visible'
export interface PendingEvent {
  taskId: string
  eventId: string
  eventType: EventType
  clientAt: string
}
const types = ['copy', 'paste', 'tab_hidden', 'tab_visible']
export const queueLimit = 500
const day = 24 * 60 * 60 * 1000

export function restoreEvents(raw: unknown, now: number): PendingEvent[] {
  if (!Array.isArray(raw)) return []
  const ids = new Set<string>()
  return raw.filter((event): event is PendingEvent => {
    if (!event || typeof event.taskId !== 'string' || !event.taskId
      || typeof event.eventId !== 'string' || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(event.eventId)
      || !types.includes(event.eventType) || typeof event.clientAt !== 'string'
      || !Number.isFinite(Date.parse(event.clientAt))
      || now - Date.parse(event.clientAt) >= day || ids.has(event.eventId)) return false
    ids.add(event.eventId)
    return true
  }).slice(0, queueLimit)
}

export function retryAfterMs(value: string | null, now = Date.now()): number {
  if (value === null || value.trim() === '') return 60000
  const seconds = Number(value)
  if (Number.isFinite(seconds)) return Math.max(1000, seconds * 1000)
  const date = Date.parse(value)
  return Number.isFinite(date) ? Math.max(1000, date - now) : 60000
}

// A pending departure belongs to its original task, even if selection changes.
export function visibilityTransition(pendingTask: string | null, currentTask: string | null, hidden: boolean): {
  pendingTask: string | null
  event: { taskId: string; eventType: EventType } | null
} {
  if (hidden) {
    if (pendingTask || !currentTask) return { pendingTask, event: null }
    return { pendingTask: currentTask, event: { taskId: currentTask, eventType: 'tab_hidden' } }
  }
  if (!pendingTask) return { pendingTask: null, event: null }
  return { pendingTask: null, event: { taskId: pendingTask, eventType: 'tab_visible' } }
}
