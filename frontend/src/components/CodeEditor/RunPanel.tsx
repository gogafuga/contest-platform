import { useEffect, useRef, useState } from 'react'
import { ApiError, createCodeRun, getCodeRun, type CodeRun } from '../../lib/api'
import type { Language } from '../../lib/languages'
import styles from './CodeEditor.module.css'

interface Pending { requestId: string; sourceCode?: string; language?: Language; id?: string; createdAt: number }
const active = (run: CodeRun | null) => run?.status === 'queued' || run?.status === 'running'
function uuid() {
  const bytes = crypto.getRandomValues(new Uint8Array(16)); bytes[6] = bytes[6] & 15 | 64; bytes[8] = bytes[8] & 63 | 128
  const value = Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('')
  return `${value.slice(0, 8)}-${value.slice(8, 12)}-${value.slice(12, 16)}-${value.slice(16, 20)}-${value.slice(20)}`
}
export default function RunPanel({ taskId, participantId, token, disabled, snapshot, prepare, onPassed, onRunChange, onShowOutput }: {
  taskId: string; participantId: number; token: string; disabled: boolean
  snapshot: () => { sourceCode: string; language: Language }
  prepare: (source: string, language: Language) => Promise<boolean>
  onPassed: (earned: number) => void
  onRunChange: (run: CodeRun) => void
  onShowOutput: () => void
}) {
  const key = `devsec_run:${participantId}:${taskId}`
  const [pending, setPending] = useState<Pending | null>(() => {
    try { const value = JSON.parse(localStorage.getItem(key) ?? 'null'); return value && typeof value.requestId === 'string' && Date.now() - value.createdAt < 86400000 ? value : null } catch { return null }
  })
  const [run, setRun] = useState<CodeRun | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const passed = useRef(onPassed); passed.current = onPassed
  const announced = useRef('')
  const publish = useRef(onRunChange); publish.current = onRunChange
  useEffect(() => { if (run) publish.current(run) }, [run])
  function remember(value: Pending | null) {
    setPending(value)
    try { if (value) localStorage.setItem(key, JSON.stringify(value)); else localStorage.removeItem(key) } catch { /* Live session still works without local persistence. */ }
  }
  const runId = pending?.id
  useEffect(() => {
    if (!runId) return
    let alive = true
    let timer: number | undefined
    async function poll() {
      try {
        const row = await getCodeRun(taskId, token, runId!)
        if (!alive) return
        setRun(row); setError('')
        if (!active(row)) {
          // Keep the last ID in storage so the completed result survives reload.
          setPending(null)
          if (row.passed && announced.current !== row.id) { announced.current = row.id; passed.current(row.earned) }
          return
        }
        timer = window.setTimeout(() => { void poll() }, 750)
      } catch (err) {
        if (!alive) return
        if (err instanceof ApiError && [400, 401, 403, 404].includes(err.status)) {
          remember(null); setError(err.message); return
        }
        setError('Не удалось получить результат. Повторим запрос автоматически.')
        timer = window.setTimeout(() => { void poll() }, 2000)
      }
    }
    void poll()
    return () => { alive = false; window.clearTimeout(timer) }
    // Persistence is scoped to this mounted participant/task editor.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [runId, taskId, token])

  async function start() {
    setSubmitting(true); setError('')
    try {
      let request = pending
      if (!request || request.id) {
        const code = snapshot()
        if (!code.sourceCode.trim()) { setError('Сначала напишите код.'); return }
        if (!await prepare(code.sourceCode, code.language)) { setError('Не удалось сохранить код перед запуском. Проверьте статус сохранения.'); return }
        request = { requestId: uuid(), ...code, createdAt: Date.now() }
        remember(request)
      }
      if (!request.sourceCode || !request.language) { setError('Не удалось восстановить запрос запуска.'); return }
      const row = await createCodeRun(taskId, token, request.requestId, request.language, request.sourceCode)
      setRun(row)
      remember({ requestId: request.requestId, id: row.id, createdAt: request.createdAt })
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message)
        if ([400, 401, 403, 404, 409, 429].includes(err.status)) remember(null)
      } else setError('Ответ сервера не получен. Нажмите «Уточнить запуск»: повторный запрос не создаст вторую отправку.')
    } finally { setSubmitting(false) }
  }
  const running = active(run) || !!runId
  return <section className={styles.runPanel} aria-label="Выполнение и проверка">
    <div className={styles.runToolbar}>
      <button type="button" className={styles.runButton} disabled={disabled || submitting || running} onClick={() => { void start() }}>
        {submitting ? 'Отправка…' : running ? run?.status === 'running' ? 'Компиляция и выполнение…' : 'В очереди…' : pending && !pending.id ? 'Уточнить запуск' : '▶ Выполнить и проверить'}
      </button>
      <span className={styles.hint}>Правильный ответ начисляет баллы.</span>
    </div>
    {error && <p className={styles.runError} role="alert">{error}</p>}
    {run && <button type="button" className={styles.outputButton} onClick={onShowOutput}>{!active(run) ? run.passed ? '✓ Решено — открыть вывод ↗' : 'Результат и вывод программы ↗' : 'Открыть вывод программы ↗'}</button>}
  </section>
}
