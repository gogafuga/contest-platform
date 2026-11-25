import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useContest } from '../../state/contest'
import { register as apiRegister, ApiError } from '../../lib/api'
import styles from './Register.module.css'

export default function Register() {
  const { register } = useContest()
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [group, setGroup] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const trimmed = name.trim()
    if (trimmed.length < 2) {
      setError('Введите полное имя — имя и фамилию.')
      return
    }
    if (!group.trim()) {
      setError('Укажите группу или класс.')
      return
    }

    setSubmitting(true)
    setError(null)
    try {
      const { token, participant } = await apiRegister(trimmed, group.trim())
      register(token, participant)
      navigate('/tasks')
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setError('Это имя уже зарегистрировано.')
      } else if (err instanceof ApiError) {
        setError(err.message)
      } else {
        setError('Не удалось подключиться к серверу.')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className={styles.page}>
      <form className={styles.card} onSubmit={handleSubmit}>
        <span className={styles.eyebrow}>Кодинг-контест</span>
        <h1 className={styles.title}>DevSec Contest01</h1>

        <label className={styles.field}>
          <span>Полное имя</span>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Иванов Иван Иванович"
            autoComplete="name"
            autoFocus
          />
        </label>

        <label className={styles.field}>
          <span>Группа</span>
          <input
            type="text"
            value={group}
            onChange={(e) => setGroup(e.target.value)}
            placeholder="20.41"
          />
        </label>

        {error && <p className={styles.error}>{error}</p>}

        <button type="submit" className={styles.submit} disabled={submitting}>
          {submitting ? 'Входим…' : 'Войти →'}
        </button>
      </form>
    </div>
  )
}
