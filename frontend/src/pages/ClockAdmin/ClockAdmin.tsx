import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import CountdownTimer from '../../components/CountdownTimer/CountdownTimer'
import { startClock, ApiError, type ClockState } from '../../lib/api'
import styles from './ClockAdmin.module.css'

const ADMIN_TOKEN_KEY = 'devsec_admin_token'
const DURATION_MINUTES_KEY = 'devsec_admin_duration_minutes'
const DEFAULT_DURATION_MINUTES = 90
const MIN_DURATION_MINUTES = 1
const MAX_DURATION_MINUTES = 14 * 24 * 60

export default function ClockAdmin() {
  const [token, setToken] = useState(() => localStorage.getItem(ADMIN_TOKEN_KEY) ?? '')
  const [authorized, setAuthorized] = useState(() => Boolean(localStorage.getItem(ADMIN_TOKEN_KEY)))
  const [durationMinutes, setDurationMinutes] = useState(
    () => localStorage.getItem(DURATION_MINUTES_KEY) ?? String(DEFAULT_DURATION_MINUTES),
  )
  const [error, setError] = useState<string | null>(null)
  const [starting, setStarting] = useState(false)
  const [startedClock, setStartedClock] = useState<ClockState | null>(null)
  const [timerVersion, setTimerVersion] = useState(0)

  const handleAuth = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (!token.trim()) {
      setError('Введите токен.')
      return
    }
    setError(null)
    setAuthorized(true)
  }

  const startOrRestart = async () => {
    const minutes = Number(durationMinutes)
    if (!Number.isFinite(minutes) || minutes < MIN_DURATION_MINUTES || minutes > MAX_DURATION_MINUTES) {
      setError(`Длительность — от ${MIN_DURATION_MINUTES} до ${MAX_DURATION_MINUTES} минут.`)
      return
    }

    setStarting(true)
    setError(null)
    try {
      const clock = await startClock(token, Math.round(minutes * 60))
      setStartedClock(clock)
      setTimerVersion((version) => version + 1)
      localStorage.setItem(ADMIN_TOKEN_KEY, token)
      localStorage.setItem(DURATION_MINUTES_KEY, durationMinutes)
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setError('Неверный токен.')
        setAuthorized(false)
        localStorage.removeItem(ADMIN_TOKEN_KEY)
      } else if (err instanceof ApiError) {
        setError(err.message)
      } else {
        setError('Не удалось связаться с сервером.')
      }
    } finally {
      setStarting(false)
    }
  }

  return (
    <div className={styles.page}>
      {!authorized ? (
        <form className={styles.card} onSubmit={handleAuth}>
          <span className={styles.eyebrow}>Управление раундом</span>
          <h1 className={styles.title}>Таймер</h1>
          <Link to="/history">История кода участников</Link>
          <label className={styles.field}>
            <span>Админ-токен</span>
            <input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              autoFocus
            />
          </label>
          {error && <p className={styles.error}>{error}</p>}
          <button type="submit" className={styles.submit}>
            Войти
          </button>
        </form>
      ) : (
        <div className={styles.card}>
          <span className={styles.eyebrow}>Управление раундом</span>
          <h1 className={styles.title}>Таймер</h1>
          <Link to="/history">История кода участников</Link>
          <div className={styles.timerRow}>
            <CountdownTimer key={timerVersion} initialClock={startedClock} />
          </div>
          <label className={styles.field}>
            <span>Длительность, мин</span>
            <input
              type="number"
              min={MIN_DURATION_MINUTES}
              max={MAX_DURATION_MINUTES}
              value={durationMinutes}
              onChange={(e) => setDurationMinutes(e.target.value)}
            />
          </label>
          {error && <p className={styles.error}>{error}</p>}
          <button type="button" className={styles.submit} onClick={startOrRestart} disabled={starting}>
            {starting ? 'Запуск…' : 'Запустить / перезапустить таймер'}
          </button>
          <p className={styles.note}>
            Виден на странице «Задачи» у всех участников.
          </p>
        </div>
      )}
    </div>
  )
}
