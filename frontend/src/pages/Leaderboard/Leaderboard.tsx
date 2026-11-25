import { useEffect, useState } from 'react'
import Header from '../../components/Header/Header'
import { getLeaderboard, type LeaderboardEntry, type TaskSpec } from '../../lib/api'
import { useContest } from '../../state/contest'
import styles from './Leaderboard.module.css'

const POLL_MS = 4000

export default function Leaderboard() {
  const { participant } = useContest()
  const [entries, setEntries] = useState<LeaderboardEntry[]>([])
  const [tasks, setTasks] = useState<TaskSpec[]>([])

  useEffect(() => {
    let cancelled = false
    const poll = () => {
      getLeaderboard()
        .then((res) => {
          if (!cancelled) {
            setEntries(res.entries)
            setTasks(res.tasks)
          }
        })
        .catch(() => {
          // transient network hiccup — keep showing the last known table
        })
    }
    poll()
    const id = window.setInterval(poll, POLL_MS)
    return () => {
      cancelled = true
      window.clearInterval(id)
    }
  }, [])

  const ranked = [...entries].sort((a, b) => b.total - a.total)

  return (
    <div className={styles.page}>
      <Header />
      <main className={styles.main}>
        <h2 className={styles.heading}>Таблица лидеров</h2>
        <div className={styles.tableWrap}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th>#</th>
                <th>Участник</th>
                <th>Группа</th>
                {tasks.map((t) => (
                  <th key={t.id} className="tnum">
                    {t.code}
                  </th>
                ))}
                <th className="tnum">Итого</th>
              </tr>
            </thead>
            <tbody>
              {ranked.map((entry, i) => {
                const isYou =
                  participant !== null &&
                  entry.name.toLowerCase() === participant.name.toLowerCase() &&
                  entry.group.toLowerCase() === participant.group.toLowerCase()
                return (
                  <tr key={`${entry.name}-${entry.group}`} className={isYou ? styles.you : undefined}>
                    <td className="tnum">{i + 1}</td>
                    <td>
                      {entry.name}
                      {isYou && <span className={styles.youTag}>вы</span>}
                    </td>
                    <td className={styles.groupCell}>{entry.group}</td>
                    {tasks.map((t) => (
                      <td key={t.id} className="tnum">
                        {entry.scores[t.id] !== undefined
                          ? Math.round(entry.scores[t.id] * 100) / 100
                          : '—'}
                      </td>
                    ))}
                    <td className={`${styles.totalCell} tnum`}>
                      {Math.round(entry.total * 100) / 100}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </main>
    </div>
  )
}
