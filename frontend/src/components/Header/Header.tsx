import { NavLink } from 'react-router-dom'
import CountdownTimer from '../CountdownTimer/CountdownTimer'
import { useContest } from '../../state/contest'
import styles from './Header.module.css'

export default function Header() {
  const { participant, total } = useContest()

  return (
    <header className={styles.header}>
      <div className={styles.left}>
        <div className={styles.identity}>
          <span className={styles.eyebrow}>Кодинг-контест</span>
          <h1 className={styles.title}>DevSec Contest_01</h1>
        </div>

        <nav className={styles.nav}>
          <NavLink
            to="/tasks"
            className={({ isActive }) => (isActive ? `${styles.link} ${styles.active}` : styles.link)}
          >
            Задачи
          </NavLink>
          <NavLink
            to="/leaderboard"
            className={({ isActive }) => (isActive ? `${styles.link} ${styles.active}` : styles.link)}
          >
            Таблица лидеров
          </NavLink>
        </nav>
      </div>

      <div className={styles.center}>
        <CountdownTimer />
      </div>

      <div className={styles.right}>
        {participant && (
          <div className={styles.participant}>
            <span className={styles.pName}>
              {participant.name} <span className={styles.pGroup}>· {participant.group}</span>
            </span>
            <span className={`${styles.pScore} tnum`}>{Math.round(total * 100) / 100} баллов</span>
          </div>
        )}
      </div>
    </header>
  )
}
