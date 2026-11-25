import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import { codeTemplates, inputExamples, outputExamples } from '../../lib/codeTemplates'
import { languages, isLanguage, type Language } from '../../lib/languages'
const Monaco = lazy(() => import('../Monaco/Monaco'))
import { ApiError, getDraft, saveDraft, type CodeRun } from '../../lib/api'
import styles from './CodeEditor.module.css'
import RunPanel from './RunPanel'
import { useContest } from '../../state/contest'

const MAX_BYTES = 256 * 1024

export default function CodeEditor({ taskId, token, participantId, readOnly, onRunChange, onShowOutput }: {
  taskId: string
  token: string
  participantId: number
  readOnly: boolean
  onRunChange: (run: CodeRun) => void
  onShowOutput: () => void
}) {
  const { setScore } = useContest()
  const initialReadOnly = useRef(readOnly)
  const saving = useRef<Promise<boolean> | null>(null)
  const [code, setCode] = useState('')
  const [guide, setGuide] = useState<'read' | 'write' | null>(null)
  const [language, setLanguage] = useState<Language>('go')
  const currentLanguage = useRef<Language>('go')
  const savedLanguage = useRef<Language>('go')
  const [status, setStatus] = useState('Загрузка черновика…')
  const [assistStatus, setAssistStatus] = useState('')
  const [ready, setReady] = useState(false)
  const [blocked, setBlocked] = useState(false)
  const [retry, setRetry] = useState(0)
  const current = useRef('')
  const saved = useRef('')
  const version = useRef(0)
  const busy = useRef(false)
  // The session token is deliberately not persisted in the backup key.
  const backupKey = useRef('')

  useEffect(() => {
    let active = true
    async function load() {
      try {
        const key = `devsec_draft:${participantId}:${taskId}`
        const draft = await getDraft(taskId, token)
        if (!active) return
        backupKey.current = key
        version.current = draft.version
        saved.current = draft.sourceCode
        savedLanguage.current = draft.language ?? 'go'
        let text = draft.sourceCode
        let lang: Language = draft.language ?? 'go'
        let conflict = false
        try {
          const raw = localStorage.getItem(key)
          const backup = raw ? JSON.parse(raw) : null
          if (backup && typeof backup.code === 'string' && Number.isSafeInteger(backup.version)) {
            text = backup.code
            lang = isLanguage(backup.language) ? backup.language : 'go'
            conflict = (text !== draft.sourceCode || lang !== savedLanguage.current) && backup.version !== draft.version
          }
        } catch { /* Server copy remains available if local storage is unavailable. */ }
        if (!text.trim() && !initialReadOnly.current) text = codeTemplates[lang]
        current.current = text
        currentLanguage.current = lang
        setLanguage(lang)
        setCode(text)
        setBlocked(conflict)
        setReady(true)
        setStatus(conflict
          ? 'Локальный код отличается от новой версии на сервере. Скачайте его перед загрузкой серверной версии.'
          : text === draft.sourceCode && lang === savedLanguage.current ? 'Сохранено' : 'Восстановлен локальный черновик')
      } catch {
        if (active) setStatus('Не удалось загрузить черновик. Повторите загрузку.')
      }
    }
    void load()
    return () => { active = false }
  }, [taskId, token, participantId, retry])

  const saveSnapshot = useCallback(async (text: string, lang: Language): Promise<boolean> => {
    while (saving.current) { if (!await saving.current) return false }
    if (text === saved.current && lang === savedLanguage.current) return true
    if (new TextEncoder().encode(text).length > MAX_BYTES) { setStatus('Код превышает лимит 256 КиБ.'); return false }
    busy.current = true
    const request = (async () => {
      setStatus('Сохраняется…')
      try {
        const draft = await saveDraft(taskId, token, text, version.current, lang)
        version.current = draft.version; saved.current = text; savedLanguage.current = lang
        try {
          if (current.current === text && currentLanguage.current === lang) localStorage.removeItem(backupKey.current)
          else localStorage.setItem(backupKey.current, JSON.stringify({ code: current.current, language: currentLanguage.current, version: draft.version }))
        } catch { /* Server copy was saved successfully. */ }
        setStatus(current.current === text && currentLanguage.current === lang ? 'Сохранено' : 'Есть несохранённые изменения')
        return true
      } catch (err) {
        if (err instanceof ApiError && err.status === 409) { setBlocked(true); setStatus('Конфликт версий. Скачайте свой код перед загрузкой серверной версии.') }
        else if (err instanceof ApiError && [400, 401, 403, 404, 413].includes(err.status)) { setBlocked(true); setStatus(`Сохранение остановлено (${err.status}).`) }
        else setStatus('Нет связи с сервером. Повторим сохранение автоматически.')
        return false
      }
    })()
    saving.current = request
    try { return await request } finally { if (saving.current === request) saving.current = null; busy.current = false }
  }, [taskId, token])

  useEffect(() => {
    if (!ready || blocked || readOnly) return
    const timer = window.setInterval(() => {
      if (!busy.current) void saveSnapshot(current.current, currentLanguage.current)
    }, 1000)
    return () => window.clearInterval(timer)
  }, [ready, blocked, readOnly, saveSnapshot])

  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => {
      if ((current.current !== saved.current || currentLanguage.current !== savedLanguage.current)) event.preventDefault()
    }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [])

  function edit(text: string) {
    current.current = text
    setCode(text)
    try {
      localStorage.setItem(backupKey.current, JSON.stringify({ code: text, language: currentLanguage.current, version: version.current }))
      if (!blocked) setStatus('Есть несохранённые изменения')
    } catch {
      setStatus('Локальная копия недоступна. Не закрывайте страницу до сохранения на сервере.')
    }
  }

  function download() {
    const url = URL.createObjectURL(new Blob([current.current], { type: 'text/plain;charset=utf-8' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `${taskId}.${languages.find(item => item.id === currentLanguage.current)!.extension}`
    a.click()
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
  }

  function reload() {
    if (ready && !window.confirm('Загрузить серверную версию? Локальные изменения будут заменены. Сначала скачайте их, если хотите сохранить.')) return
    try { if (backupKey.current) localStorage.removeItem(backupKey.current) } catch { return }
    setReady(false)
    setBlocked(false)
    setStatus('Загрузка черновика…')
    setRetry(n => n + 1)
  }

  return (
    <section className={styles.editor} aria-label="Черновик программы">
      <div className={styles.toolbar}>
        <div className={styles.languageControl}>
        <label htmlFor={`language-${taskId}`}>Язык:</label>
        <select id={`language-${taskId}`} value={language} disabled={!ready || readOnly} onChange={e => {
          const next = e.target.value
          if (!isLanguage(next)) return
          const old = currentLanguage.current
          const replace = !current.current.trim() || current.current === codeTemplates[old]
          if (!replace && !window.confirm('Заменить код стартовым шаблоном выбранного языка? Сначала скачайте код, если хотите его сохранить.')) return
          currentLanguage.current = next
          setLanguage(next)
          edit(codeTemplates[next])
        }}>
          {languages.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}
        </select>
        </div>
        <div className={styles.toolbarStatus}>
          <span id={`status-${taskId}`} role="status" className={styles.status} title={status}>{status}</span>
          {!readOnly && assistStatus && <>
            <span className={styles.statusSeparator} aria-hidden="true">·</span>
            <span className={assistStatus === 'Подсказки подключены' ? styles.assistReady : styles.assistWarning} role="status" title={assistStatus}>{assistStatus}</span>
          </>}
        </div>
        <button type="button" onClick={download} disabled={!ready}>Скачать код</button>
        {(!ready || blocked) && <button type="button" onClick={reload}>Загрузить с сервера</button>}
      </div>
      <aside className={styles.inputGuide} aria-label="Работа с входными данными и ответом">
        <div className={styles.guideRow}>
          <span><code>input.txt</code> → файл ответа <code>stdout</code></span>
          <button type="button" aria-expanded={guide === 'read'} aria-controls={`guide-${taskId}`} onClick={() => setGuide(guide === 'read' ? null : 'read')}>Чтение</button>
          <button type="button" aria-expanded={guide === 'write'} aria-controls={`guide-${taskId}`} onClick={() => setGuide(guide === 'write' ? null : 'write')}>Запись</button>
        </div>
        {guide && <div id={`guide-${taskId}`} className={styles.guideExample}>
          <strong>{guide === 'read' ? 'Как прочитать input.txt' : 'Как создать файл stdout'} · {languages.find(item => item.id === language)!.label}</strong>
          <pre>{guide === 'read' ? inputExamples[language] : outputExamples[language]}</pre>
          <p>{guide === 'read'
            ? 'В переменной data — содержимое входного файла. Обработайте его по условию задачи.'
            : 'Создаётся файл stdout без расширения в рабочем каталоге.'}</p>
        </div>}
      </aside>
      <div className={styles.canvas}>
        {ready && <Suspense fallback={<p>Загрузка редактора…</p>}>
          <Monaco height="100%" value={code} language={language} onChange={edit} readOnly={readOnly} assist={{ taskId, token }} onAssistStatus={setAssistStatus} />
        </Suspense>}
      </div>
      <RunPanel taskId={taskId} participantId={participantId} token={token} disabled={!ready || readOnly || blocked}
        snapshot={() => ({ sourceCode: current.current, language: currentLanguage.current })}
        prepare={saveSnapshot} onRunChange={onRunChange} onShowOutput={onShowOutput} onPassed={earned => setScore(taskId, earned)} />
    </section>
  )
}
