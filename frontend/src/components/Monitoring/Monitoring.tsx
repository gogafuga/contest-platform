import { useEffect, useRef, useState } from 'react'
import { sendParticipantEvent, type EventType, type PendingEvent } from '../../lib/api'
import styles from './Monitoring.module.css'
import { restoreEvents, retryAfterMs, queueLimit, visibilityTransition } from '../../lib/eventQueue'

// Scope local data to the session without putting its bearer token in storage keys.
function sessionKey(token: string) {
  let hash = 2166136261
  for (const char of token) hash = Math.imul(hash ^ char.charCodeAt(0), 16777619)
  return (hash >>> 0).toString(16)
}
function uuid() {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  bytes[6] = (bytes[6] & 15) | 64
  bytes[8] = (bytes[8] & 63) | 128
  const hex = Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

export default function Monitoring({ token, participantId, taskId }: {
  token: string; participantId: number; taskId: string | null
}) {
  const scope = `devsec_monitor:v1:${participantId}:${sessionKey(token)}`
  const [accepted, setAccepted] = useState(() => {
    try { return localStorage.getItem(`${scope}:notice`) === 'seen' } catch { return false }
  })
  const [status, setStatus] = useState('')
  const [noticeOpen, setNoticeOpen] = useState(false)
  const task = useRef(taskId)
  const syncVisibility = useRef<(() => void) | null>(null)
  useEffect(() => { task.current = taskId; syncVisibility.current?.() }, [taskId])

  useEffect(() => {
    if (!accepted) return
    let alive = true
    let busy = false
    let nextAttempt = 0
    let delay = 2000
    let queue: PendingEvent[] = []
    let storageKey = `${scope}:queue`
    try {
      let tabId = sessionStorage.getItem('devsec_monitor_tab')
      if (!tabId) { tabId = uuid(); sessionStorage.setItem('devsec_monitor_tab', tabId) }
      storageKey += `:${tabId}`
      const raw: unknown = JSON.parse(localStorage.getItem(storageKey) ?? '[]')
      queue = restoreEvents(raw, Date.now())
    } catch { setStatus('Локальная очередь недоступна: при закрытии страницы события могут потеряться.') }

    function persist() {
      try {
        if (queue.length) localStorage.setItem(storageKey, JSON.stringify(queue))
        else localStorage.removeItem(storageKey)
      } catch {
        if (alive) setStatus('Не удалось сохранить очередь локально. Не закрывайте страницу до восстановления связи.')
      }
    }
    persist()
    async function flush() {
      if (!alive || busy || !queue.length || Date.now() < nextAttempt) return
      busy = true
      const event = queue[0]
      try {
        const response = await sendParticipantEvent(event, token)
        if (!alive) return
        if (response.status === 204) {
          queue.shift()
          delay = 2000
          nextAttempt = Date.now() + 1000
          persist()
          setStatus(queue.length ? `События ожидают отправки: ${queue.length}` : '')
        } else if (response.status === 429) {
          nextAttempt = Date.now() + retryAfterMs(response.headers.get('Retry-After'))
          setStatus('Отправка событий отложена из-за ограничения частоты.')
        } else if (response.status === 401) {
          nextAttempt = Infinity
          setStatus('Сессия истекла. Обновите страницу и войдите заново; очередь пока сохранена локально.')
        } else if ([400, 403, 404, 413].includes(response.status)) {
          queue.shift()
          persist()
          nextAttempt = Date.now() + 1000
          setStatus(`Сервер отклонил событие (${response.status}). Оно не записано.`)
        } else {
          throw new Error(`event request: ${response.status}`)
        }
      } catch {
        if (alive) {
          nextAttempt = Date.now() + delay
          delay = Math.min(delay * 2, 60000)
          setStatus(`События ожидают связи с сервером: ${queue.length}`)
        }
      } finally { busy = false }
    }
    function record(eventType: EventType, eventTask = task.current): boolean {
      if (!eventTask) return false
      if (queue.length >= queueLimit) {
        setStatus('Очередь событий заполнена. Новые события не записываются до восстановления связи.')
        return false
      }
      queue.push({ taskId: eventTask, eventId: uuid(), eventType, clientAt: new Date().toISOString() })
      persist()
      void flush()
      return true
    }
    const copy = () => record('copy')
    const paste = () => record('paste')
    const visibilityKey = `${scope}:visibility`
    let pendingTask: string | null = null
    try { pendingTask = sessionStorage.getItem(visibilityKey) } catch { /* Track in memory. */ }
    const visibility = () => {
      const transition = visibilityTransition(pendingTask, task.current, document.hidden)
      if (transition.event && !record(transition.event.eventType, transition.event.taskId)) return
      pendingTask = transition.pendingTask
      try {
        if (pendingTask) sessionStorage.setItem(visibilityKey, pendingTask)
        else sessionStorage.removeItem(visibilityKey)
      } catch { /* In-memory tracking still works until the page is reloaded. */ }
    }
    syncVisibility.current = visibility
    const online = () => { void flush() }
    document.addEventListener('copy', copy, true)
    document.addEventListener('paste', paste, true)
    document.addEventListener('visibilitychange', visibility)
    window.addEventListener('online', online)
    const timer = window.setInterval(() => { void flush() }, 1000)
    visibility()
    void flush()
    return () => {
      syncVisibility.current = null
      alive = false
      window.clearInterval(timer)
      document.removeEventListener('copy', copy, true)
      document.removeEventListener('paste', paste, true)
      document.removeEventListener('visibilitychange', visibility)
      window.removeEventListener('online', online)
    }
  }, [accepted, scope, token])

  function acknowledge() {
    try { localStorage.setItem(`${scope}:notice`, 'seen') } catch { /* Notice will be shown again on reload. */ }
    setAccepted(true)
    setNoticeOpen(false)
  }
  return <>
    <aside className={styles.bar} aria-label="Мониторинг соревнования">
      <span>Фиксируются копирование, вставка и скрытие/возврат вкладки для выбранной задачи.</span>
      <button type="button" onClick={() => setNoticeOpen(true)}>Подробнее</button>
      {status && <span role="status">{status}</span>}
    </aside>
    {(!accepted || noticeOpen) && <div className={styles.overlay}>
      <section className={styles.notice} role="dialog" aria-modal="true" aria-labelledby="monitor-title">
        <h2 id="monitor-title">Уведомление о мониторинге</h2>
        <p>Для выбранной задачи сайт сохраняет факты копирования и вставки, скрытия вкладки и возврата, время браузера и время приёма сервером. История сохраняемого кода также доступна преподавателю.</p>
        <p>Содержимое буфера обмена и действия в других приложениях не собираются. Эти события сами по себе не доказывают списывание.</p>
        <p>Ожидающие отправки события временно хранятся в этом браузере, до 500 на вкладку; при восстановлении страницы записи старше суток удаляются. Серверные записи доступны через API преподавателя. Срок их хранения организатор должен определить до реального соревнования.</p>
        <p>Мониторинг начинается после ознакомления с этим уведомлением.</p>
        <button type="button" autoFocus onClick={acknowledge}>{accepted ? 'Закрыть' : 'Ознакомлен, продолжить'}</button>
      </section>
    </div>}
  </>
}
