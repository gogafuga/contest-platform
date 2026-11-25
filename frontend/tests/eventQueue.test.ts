import assert from 'node:assert/strict'
import test from 'node:test'
import { restoreEvents, retryAfterMs, queueLimit, visibilityTransition, type PendingEvent } from '../src/lib/eventQueue.ts'

const now = Date.parse('2026-11-23T09:00:00Z')
function event(index = 1): PendingEvent {
  return {
    taskId: 'noisy-ips', eventId: `00000000-0000-4000-8000-${String(index).padStart(12, '0')}`,
    eventType: 'paste', clientAt: new Date(now).toISOString(),
  }
}
test('offline restore preserves event IDs and order, rejecting duplicates and malformed or expired records', () => {
  const first = event(1), second = event(2)
  const restored = restoreEvents([
    first, first, null, {}, { ...event(3), eventType: 'clipboard_text' },
    { ...event(4), clientAt: 'bad' }, { ...event(5), eventId: 'invalid' },
    { ...event(6), clientAt: new Date(now - 86400000).toISOString() }, second,
  ], now)
  assert.deepEqual(restored, [first, second])
  assert.deepEqual(restoreEvents('bad', now), [])
})
test('offline queue restore is bounded', () => {
  assert.equal(restoreEvents(Array.from({length: 600}, (_, i) => event(i)), now).length, queueLimit)
})
test('429 retry waits support seconds and HTTP dates without immediate retry loops', () => {
  assert.equal(retryAfterMs('60', now), 60000)
  assert.equal(retryAfterMs(null, now), 60000)
  assert.equal(retryAfterMs('garbage', now), 60000)
  assert.equal(retryAfterMs('0', now), 1000)
  assert.equal(retryAfterMs(new Date(now + 30000).toUTCString(), now), 30000)
})

test('visibility ignores repeated hidden notifications and restores return after remount', () => {
  const departure = visibilityTransition(null, 'task-a', true)
  assert.equal(departure.event?.eventType, 'tab_hidden')
  assert.equal(visibilityTransition(departure.pendingTask, 'task-a', true).event, null)
  // The pending task is restored from sessionStorage by the component after reload.
  const returned = visibilityTransition(departure.pendingTask, 'task-b', false)
  assert.deepEqual(returned.event, { taskId: 'task-a', eventType: 'tab_visible' })
  assert.equal(visibilityTransition(returned.pendingTask, 'task-b', false).event, null)
  assert.equal(visibilityTransition(null, 'task-a', false).event, null)
  assert.equal(visibilityTransition(null, null, true).event, null)
})
