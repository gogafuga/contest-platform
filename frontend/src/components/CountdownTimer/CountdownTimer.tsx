import { useEffect, useRef, useState } from 'react'
import { getClock, type ClockState } from '../../lib/api'
import { formatCountdown } from './format'
import styles from './CountdownTimer.module.css'

// While waiting for the admin to start (or after the contest ends), poll
// often so the transition shows up quickly — jittered so many browsers
// hitting /clock at once (e.g. everyone's page loaded right before start)
// don't all land on the backend in the same instant, repeatedly.
const WAITING_POLL_BASE_MS = 1500
const WAITING_POLL_JITTER_MS = 1000

// Once running, the countdown is computed locally from the last known
// startedAt/durationSeconds; only resync occasionally, to correct
// client/server clock drift and to notice an admin restart.
const RUNNING_RESYNC_BASE_MS = 30000
const RUNNING_RESYNC_JITTER_MS = 10000

function jittered(base: number, jitter: number) {
  return base + Math.random() * jitter
}

/** Read-only display — the clock is started/restarted only from /clock (admin-gated), server-side. */
export default function CountdownTimer({ initialClock = null }: { initialClock?: ClockState | null }) {
  const [clock, setClock] = useState<ClockState | null>(initialClock)
  const [now, setNow] = useState(() => Date.now())
  const clockRef = useRef<ClockState | null>(initialClock)

  useEffect(() => {
    let cancelled = false
    let timeoutId: number

    const scheduleNext = () => {
      const delay =
        clockRef.current?.status === 'running'
          ? jittered(RUNNING_RESYNC_BASE_MS, RUNNING_RESYNC_JITTER_MS)
          : jittered(WAITING_POLL_BASE_MS, WAITING_POLL_JITTER_MS)
      timeoutId = window.setTimeout(poll, delay)
    }

    const poll = () => {
      getClock()
        .then((state) => {
          if (cancelled) return
          clockRef.current = state
          setClock(state)
        })
        .catch(() => {
          // transient network hiccup — keep the last known state, retry on the usual cadence
        })
        .finally(() => {
          if (!cancelled) scheduleNext()
        })
    }

    poll()
    const tickId = window.setInterval(() => setNow(Date.now()), 500)
    return () => {
      cancelled = true
      window.clearTimeout(timeoutId)
      window.clearInterval(tickId)
    }
  }, [])

  if (!clock || clock.status === 'not_started' || !clock.startedAt) {
    return (
      <div className={styles.block}>
        <span className={styles.waiting}>ожидание старта</span>
      </div>
    )
  }

  const elapsed = (now - new Date(clock.startedAt).getTime()) / 1000
  const remaining = clock.durationSeconds - elapsed
  const expired = remaining <= 0
  const low = !expired && remaining <= 300

  return (
    <div className={styles.block}>
      <span
        className={`${styles.timer} tnum ${expired ? styles.expired : low ? styles.low : ''}`}
      >
        {formatCountdown(remaining)}
      </span>
    </div>
  )
}
