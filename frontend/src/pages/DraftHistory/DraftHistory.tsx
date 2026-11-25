import { lazy, Suspense, useEffect, useRef, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { ApiError, searchParticipants, type Participant, getDraftHistory, getDraftHistoryVersion, getParticipantEvents, getTaskTiming, type TaskTiming, type ParticipantEvent, type HistoryEntry, type HistorySnapshot } from '../../lib/api'
import { useContest } from '../../state/contest'
import styles from './DraftHistory.module.css'

const Monaco = lazy(() => import('../../components/Monaco/Monaco'))
const date = (value: string) => new Date(value).toLocaleString('ru-RU')

export default function DraftHistory() {
  const { tasks } = useContest()
  const [token, setToken] = useState('')
  const [participant, setParticipant] = useState<Participant | null>(null)
  const [taskId, setTaskId] = useState('')
  const [entries, setEntries] = useState<HistoryEntry[]>([])
  const [selected, setSelected] = useState(0)
  const [snapshot, setSnapshot] = useState<HistorySnapshot | null>(null)
  const [session, setSession] = useState<{ participantId: number; taskId: string; token: string } | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [viewLoading, setViewLoading] = useState(false)
  const generation = useRef(0)

  function chooseParticipant(student: Participant | null) {
    generation.current++
    setParticipant(student)
    setSession(null)
    setEntries([])
    setSnapshot(null)
    setLoading(false)
    setError('')
  }

  async function load(event: FormEvent) {
    event.preventDefault()
    const id = participant?.id ?? 0
    const task = taskId || tasks[0]?.id
    if (!Number.isSafeInteger(id) || id <= 0 || !task || !token.trim()) {
      setError('Введите токен, выберите студента из списка и задачу.')
      return
    }
    const request = ++generation.current
    setLoading(true)
    setError('')
    setSession(null)
    setSnapshot(null)
    setEntries([])
    try {
      const result = await getDraftHistory(id, task, token.trim())
      if (request !== generation.current) return
      setEntries(result)
      setSelected(result[0]?.version ?? 0)
      setSession({ participantId: id, taskId: task, token: token.trim() })
    } catch (err) {
      if (request === generation.current) setError(err instanceof ApiError && err.status === 401
        ? 'Неверный административный токен.' : 'Не удалось загрузить историю. Проверьте соединение и параметры.')
    } finally {
      if (request === generation.current) setLoading(false)
    }
  }

  useEffect(() => {
    if (!session || !selected) return
    let active = true
    setViewLoading(true)
    // Keep the viewer mounted while fetching: removing it shrinks the page and
    // clamps the browser scroll position when the timeline is near the bottom.
    setError('')
    getDraftHistoryVersion(session.participantId, session.taskId, selected, session.token).then(after => {
      if (!active) return
      setSnapshot(after)
    }).catch(() => {
      if (active) setError('Не удалось получить выбранную версию. Выберите её повторно или обновите список.')
    }).finally(() => { if (active) setViewLoading(false) })
    return () => { active = false }
  }, [session, selected])

  const chronological = [...entries].reverse()
  const sliderIndex = chronological.findIndex(entry => entry.version === selected)
  return (
    <main className={styles.page}>
      <div className={styles.heading}><div><p>Панель преподавателя</p><h1>История кода</h1></div><Link to="/clock">Таймер</Link><Link to="/tasks">Задачи</Link></div>
      <form onSubmit={load} className={styles.form}>
        <label>Админ-токен<input type="password" value={token} onChange={e => { setToken(e.target.value); chooseParticipant(null) }} autoComplete="off" required /></label>
        <ParticipantPicker key={token} token={token} onSelect={chooseParticipant} />
        <label>Задача<select value={taskId || tasks[0]?.id || ''} onChange={e => setTaskId(e.target.value)}>{tasks.map(task => <option key={task.id} value={task.id}>{task.title}</option>)}</select></label>
        <button disabled={loading || !tasks.length || !participant}>{loading ? 'Загрузка…' : 'Показать / обновить'}</button>
      </form>
      <p className={styles.note}>Показаны последние 100 снимков. Время отображается в часовом поясе браузера. Токен хранится только в памяти этой страницы.</p>
      {error && <p role="alert" className={styles.error}>{error}</p>}
      {session && <TaskTime key={`time:${session.participantId}:${session.taskId}:${session.token}`} session={session} />}
      {session && <EventHistory key={`${session.participantId}:${session.taskId}:${session.token}`} session={session} />}
      {session && !entries.length && <p role="status">Сохранённых версий для этого участника и задачи нет.</p>}
      {entries.length > 0 && <>
        <h2>Хронология написания кода</h2>
        <label className={styles.timeline}>
          <input type="range" aria-label="Хронология написания кода" min={0} max={Math.max(0, chronological.length - 1)} value={Math.max(0, sliderIndex)} onChange={e => setSelected(chronological[Number(e.target.value)].version)} disabled={entries.length < 2} />
        </label>
        <div className={styles.snapshotStatus} role="status">
          {viewLoading
            ? `Загрузка версии ${selected}…${snapshot ? ` Пока показана версия ${snapshot.version}.` : ''}`
            : snapshot ? `${date(snapshot.createdAt)} · ${snapshot.language}`
            : 'Выберите версию для просмотра.'}
        </div>
        <div className={`${styles.viewer} ${styles.historyViewer}`} aria-busy={viewLoading}>
          {snapshot && <Suspense fallback={<p>Загрузка редактора…</p>}>
            <Monaco height={520} value={snapshot.sourceCode} language={snapshot.language} readOnly />
          </Suspense>}
        </div>
      </>}
    </main>
  )
}

const eventNames = {
  copy: 'Копирование', paste: 'Вставка', tab_hidden: 'Ушёл со вкладки', tab_visible: 'Вернулся на вкладку',
}

function TaskTime({ session }: { session: { participantId: number; taskId: string; token: string } }) {
  const [timing, setTiming] = useState<TaskTiming | null>(null)
  const [seconds, setSeconds] = useState(0)
  const [error, setError] = useState('')
  useEffect(() => {
    const controller = new AbortController()
    let anchor = performance.now()
    let current: TaskTiming | null = null
    let busy = false
    async function refresh() {
      if (busy) return
      busy = true
      try {
        const result = await getTaskTiming(session.participantId, session.taskId, session.token, controller.signal)
        if (controller.signal.aborted) return
        current = result
        anchor = performance.now()
        setTiming(result)
        setSeconds(result.elapsedSeconds)
        setError('')
      } catch {
        if (!controller.signal.aborted) setError('Не удалось обновить время работы над задачей.')
      } finally { busy = false }
    }
    void refresh()
    const clock = window.setInterval(() => {
      if (current?.startedAt && !current.finishedAt) setSeconds(current.elapsedSeconds + Math.floor((performance.now() - anchor) / 1000))
    }, 1000)
    const poll = window.setInterval(() => { void refresh() }, 30000)
    return () => { controller.abort(); window.clearInterval(clock); window.clearInterval(poll) }
  }, [session])
  const elapsed = `${Math.floor(seconds / 3600)} ч ${Math.floor(seconds / 60) % 60} мин ${seconds % 60} с`
  return <section className={styles.events} aria-label="Время работы над задачей">
    <h2>Время на задачу: {timing?.startedAt ? `${timing.estimated ? '≈ ' : ''}${elapsed}` : timing ? 'нет данных о начале' : 'загрузка…'}</h2>
    {timing?.startedAt && <>
      <p>Начало: {date(timing.startedAt)} · {timing.finishedAt ? `Решена: ${date(timing.finishedAt)}` : 'Задача ещё не решена'}</p>
      <p className={styles.note}>{timing.estimated ? 'Начало приблизительное: по самой ранней сохранённой активности. ' : 'От выбора задачи до успешного решения. '}Перерывы входят в это время.</p>
    </>}
    {error && <p role="status" className={styles.error}>{error}</p>}
  </section>
}

function EventHistory({ session }: {
  session: { participantId: number; taskId: string; token: string }
}) {
  const [events, setEvents] = useState<ParticipantEvent[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [hasMore, setHasMore] = useState(false)
  const busy = useRef(false)
  const alive = useRef(true)
  const cursor = useRef(0)

  async function loadMore() {
    if (busy.current) return
    busy.current = true
    setLoading(true)
    setError('')
    try {
      const page = await getParticipantEvents(session.participantId, session.taskId, cursor.current, session.token)
      if (!alive.current) return
      setEvents(previous => {
        const ids = new Set(previous.map(event => event.id))
        return [...previous, ...page.filter(event => !ids.has(event.id))]
      })
      if (page.length) cursor.current = page[page.length - 1].id
      setHasMore(page.length === 100)
    } catch {
      if (alive.current) setError('Не удалось получить события. Повторите загрузку.')
    } finally {
      busy.current = false
      if (alive.current) setLoading(false)
    }
  }
  useEffect(() => {
    alive.current = true
    void loadMore()
    return () => { alive.current = false }
    // The parent remounts this component for each history session.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const counts = { copy: 0, paste: 0, tab_hidden: 0, tab_visible: 0 }
  for (const event of events) counts[event.eventType]++
  return <section className={styles.events} aria-label="События участника">
    <h2>События участника</h2>
    <p>В загруженных {events.length} событиях: копирований — {counts.copy}, вставок — {counts.paste}, уходов со вкладки — {counts.tab_hidden}, возвращений — {counts.tab_visible}.</p>
    <p className={styles.note}>Копирование учитывается только на странице соревнования. Текст из другого приложения можно вставить без события копирования на сайте, в том числе несколько раз.</p>
    {error && <p role="alert" className={styles.error}>{error}</p>}
    {!loading && !error && !events.length && <p>Событий пока нет.</p>}
    {!!events.length && <div className={styles.eventScroll}><table className={styles.eventTable}>
      <thead><tr><th>ID</th><th>Действие</th><th>Время браузера</th><th>Принято сервером</th></tr></thead>
      <tbody>{events.map(event => <tr key={event.id}>
        <td>{event.id}</td><td>{eventNames[event.eventType] ?? event.eventType}</td>
        <td>{date(event.clientAt)}</td><td>{date(event.receivedAt)}</td>
      </tr>)}</tbody>
    </table></div>}
    <button type="button" disabled={loading} onClick={() => { void loadMore() }}>{loading ? 'Загрузка событий…' : hasMore ? 'Загрузить следующие 100' : 'Проверить новые события'}</button>
  </section>
}

function ParticipantPicker({ token, onSelect }: {
  token: string
  onSelect: (participant: Participant | null) => void
}) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<Participant[]>([])
  const [chosen, setChosen] = useState<Participant | null>(null)
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [active, setActive] = useState(-1)

  useEffect(() => {
    if (!open || !token.trim()) return
    const controller = new AbortController()
    setLoading(true)
    setError('')
    setResults([])
    setActive(-1)
    const timer = window.setTimeout(() => {
      searchParticipants(query, token.trim(), controller.signal).then(participants => {
        if (!controller.signal.aborted) setResults(participants)
      }).catch(err => {
        if (!controller.signal.aborted) setError(err instanceof ApiError && err.status === 401
          ? 'Проверьте административный токен.' : 'Не удалось загрузить студентов. Повторите поиск.')
      }).finally(() => { if (!controller.signal.aborted) setLoading(false) })
    }, 300)
    return () => { controller.abort(); window.clearTimeout(timer) }
  }, [query, token, open])

  function choose(student: Participant) {
    setChosen(student)
    setQuery(student.name)
    onSelect(student)
    setOpen(false)
    setActive(-1)
  }
  return <div className={styles.picker} onBlur={event => {
    if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false)
  }}>
    <label htmlFor="student-search">ФИО студента</label>
    <input id="student-search" role="combobox" aria-autocomplete="list"
      aria-expanded={open} aria-controls="student-options"
      aria-activedescendant={open && active >= 0 ? `student-option-${active}` : undefined}
      autoComplete="off" placeholder={token.trim() ? 'Введите ФИО или часть имени' : 'Сначала введите админ-токен'}
      disabled={!token.trim()} value={query} maxLength={200}
      onFocus={() => { if (!chosen) setOpen(true) }}
      onChange={event => {
        setQuery(event.target.value)
        setChosen(null)
        onSelect(null)
        setResults([])
        setLoading(true)
        setActive(-1)
        setOpen(true)
      }}
      onKeyDown={event => {
        if (event.key === 'Escape') { setOpen(false); return }
        if (event.key === 'ArrowDown') {
          event.preventDefault(); setOpen(true)
          setActive(index => Math.min(index + 1, results.length - 1))
        } else if (event.key === 'ArrowUp') {
          event.preventDefault(); setActive(index => results.length ? Math.max(index - 1, 0) : -1)
        } else if (event.key === 'Enter' && open) {
          event.preventDefault()
          if (active >= 0 && results[active]) choose(results[active])
        }
      }} />
    {open && <div className={styles.dropdown}>
      {loading && <p role="status">Поиск…</p>}
      {error && <p role="alert" className={styles.error}>{error}</p>}
      {!loading && !error && !results.length && <p>Студенты не найдены.</p>}
      <ul id="student-options" role="listbox" aria-label="Студенты">
        {results.map((student, index) => <li id={`student-option-${index}`} key={student.id}
          role="option" aria-selected={index === active}
          className={index === active ? styles.activeOption : undefined}
          onMouseDown={event => event.preventDefault()}
          onClick={() => choose(student)}>
          <strong>{student.name}</strong><span>Группа: {student.group}</span>
        </li>)}
      </ul>
      {results.length === 20 && <p>Показаны первые 20. Уточните ФИО.</p>}
    </div>}
  </div>
}
