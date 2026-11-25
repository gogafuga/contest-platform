import { useEffect, useRef, useState } from 'react'
import type { TaskSpec, CodeRun } from '../../lib/api'
import { getTaskInput, ApiError } from '../../lib/api'
import { useContest } from '../../state/contest'
import styles from './CaseSlide.module.css'
import CodeEditor from '../CodeEditor/CodeEditor'

interface Props {
  task: TaskSpec
  focused: boolean
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

const CIPHER_CHARSET = '01#$%&@▓▒░ABCDEFGHJKMNPQRSTVWXYZабвгдежзиклмнопрстуфхцчшщ'

function cipherChar() {
  return CIPHER_CHARSET[Math.floor(Math.random() * CIPHER_CHARSET.length)]
}

function CipherField() {
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const el = containerRef.current
    if (!el) return

    let spans: HTMLSpanElement[] = []

    const build = () => {
      const rect = el.getBoundingClientRect()
      const cols = Math.max(16, Math.floor(rect.width / 8))
      const rows = Math.max(6, Math.floor(rect.height / 24))
      el.style.gridTemplateColumns = `repeat(${cols}, 1fr)`
      el.innerHTML = ''
      spans = Array.from({ length: cols * rows }, () => {
        const span = document.createElement('span')
        span.textContent = cipherChar()
        el.appendChild(span)
        return span
      })
    }

    build()
    const ro = new ResizeObserver(build)
    ro.observe(el)

    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    let intervalId: number | undefined
    if (!reduceMotion) {
      intervalId = window.setInterval(() => {
        for (let i = 0; i < 40; i++) {
          const span = spans[Math.floor(Math.random() * spans.length)]
          if (!span) continue
          span.textContent = cipherChar()
          span.classList.toggle(styles.cipherBright, Math.random() < 0.15)
        }
      }, 90)
    }

    return () => {
      ro.disconnect()
      if (intervalId) window.clearInterval(intervalId)
    }
  }, [])

  return <div ref={containerRef} className={styles.cipherField} aria-hidden="true" />
}

export default function CaseSlide({ task, focused }: Props) {
  const { token, participant, selectedTaskId, selectTask, isSolved } = useContest()
  const [downloading, setDownloading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [run, setRun] = useState<CodeRun | null>(null)
  const [leftTab, setLeftTab] = useState<'statement' | 'output'>('statement')
  const [selecting, setSelecting] = useState(false)

  const solved = isSolved(task.id)
  const isSelected = selectedTaskId === task.id
  const canSelect = selectedTaskId === null || isSolved(selectedTaskId)
  const contentUnlocked = isSelected || solved

  const handleDownload = async () => {
    if (!token) return
    setDownloading(true)
    setError(null)
    try {
      const blob = await getTaskInput(task.id, token)
      downloadBlob(blob, `${task.id}.txt`)
    } catch {
      setError('Не удалось скачать файл. Попробуйте ещё раз.')
    } finally {
      setDownloading(false)
    }
  }

  const handleSelect = async () => {
    setSelecting(true)
    setError(null)
    try {
      await selectTask(task.id)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Не удалось выбрать задачу. Попробуйте ещё раз.')
    } finally {
      setSelecting(false)
    }
  }

  const taskHeader = (
      <header className={styles.head}>
        <span className={styles.code}>{task.code}</span>
        <h2 className={styles.title}>{task.title}</h2>
        <div className={styles.taskStatus}>
        <span className={`${styles.points} tnum`}>
          {task.points} баллов
        </span>
        <span className={solved ? styles.solvedBadge : styles.unsolvedBadge}>
          {solved ? 'решено' : 'не решено'}
        </span>
        </div>
      </header>
  )

  return (
    <div className={styles.slide}>
      {!contentUnlocked && taskHeader}
      {contentUnlocked ? (
        <div className={styles.content}>
          <div className={styles.leftPane}>
          {taskHeader}
          <div className={styles.paneTabs} role="tablist" aria-label="Условие и результат">
            <button type="button" role="tab" aria-selected={leftTab === 'statement'} onClick={() => setLeftTab('statement')}>Условие задачи</button>
            <button type="button" role="tab" aria-selected={leftTab === 'output'} onClick={() => setLeftTab('output')}>Вывод программы</button>
          </div>
          <section className={styles.statement} hidden={leftTab !== 'statement'} aria-label="Условие задачи">
          <p className={styles.brief}>{task.brief}</p>
          <ul className={styles.rules}>
            {task.rules.filter(rule => !/Прикрепите|отдельный.*\.txt/i.test(rule)).concat('Прочитайте input.txt и запишите ответ построчно в файл stdout. Нажмите «Выполнить и проверить».').map((rule) => (
              <li key={rule}>{rule}</li>
            ))}
          </ul>

          <div className={styles.sampleGrid}>
            <div>
              <div className={styles.sampleLabel}>пример входа</div>
              <pre className={styles.sampleBlock}>{task.sampleInput}</pre>
            </div>

            <button
              type="button"
              className={styles.downloadBtn}
              onClick={handleDownload}
              disabled={!focused || !contentUnlocked || downloading}
            >
              {downloading ? '…' : '↓ Скачать'}
            </button>

            <div>
              <div className={styles.sampleLabel}>пример выхода</div>
              <pre className={styles.sampleBlock}>{task.sampleOutput}</pre>
            </div>
          </div>


          </section>
          <section className={styles.outputPane} hidden={leftTab !== 'output'} aria-label="Вывод программы">
            {!run ? <p>Запустите программу — результат появится здесь.</p> : <>
              <p className={run.passed ? styles.outputSuccess : styles.outputError} role="status">{run.status === 'queued' ? 'В очереди…' : run.status === 'running' ? 'Компиляция и выполнение…' : run.feedback}</p>
              {run.status !== 'queued' && run.status !== 'running' && <>
                <p className={styles.outputMeta}>Общее время компиляции и выполнения: {(run.durationMs / 1000).toFixed(2)} с · Код завершения: {run.exitCode}</p>
                {run.status === 'wrong_answer' && !run.stdout.trim() && <p className={styles.outputError}>Ответ пуст. Запишите найденные значения в файл stdout без расширения и закройте его перед завершением программы. Пример находится в разделе «Запись» над редактором.</p>}
                {run.status === 'runtime_error' && /no such file|FileNotFoundException|Could not find file|cannot find the file/i.test(run.stderr + run.stdout) && <p className={styles.outputError}>Не найден файл. Входные данные уже находятся в рабочем каталоге под именем input.txt. Читайте «input.txt» без пути с вашего компьютера.</p>}
                <h3>Ответ · stdout</h3><pre>{run.stdout || '(пусто)'}</pre>
                <h3>Ошибки · stderr</h3><pre>{run.stderr || '(пусто)'}</pre>
                {run.outputTruncated && <p>Показаны первые 64 КиБ каждого потока. Проверка использовала полный вывод в пределах лимита.</p>}
              </>}
            </>}
          </section>
          </div>
          <section className={styles.editorPane} aria-label="Редактор решения">
          {token && participant && <CodeEditor key={`${token}:${task.id}`} taskId={task.id} token={token} participantId={participant.id} readOnly={!isSelected || solved} onRunChange={setRun} onShowOutput={() => setLeftTab('output')} />}
          </section>
        </div>
      ) : (
        <div className={styles.lockable}>
          <CipherField />
          <div className={styles.lockOverlay}>
            <p className={styles.lockCaption}>
              {canSelect
                ? 'Пока вы не решите выбранную задачу, остальные будут заблокированными.'
                : 'Сейчас активна другая задача - выбор недоступен.'}
            </p>
            <button
              type="button"
              className={styles.selectBtn}
              onClick={handleSelect}
              disabled={!canSelect || selecting}
            >
              {selecting ? '…' : 'Выбрать задачу'}
            </button>
            {error && <p className={styles.lockCaption}>{error}</p>}
          </div>
        </div>
      )}
    </div>
  )
}
