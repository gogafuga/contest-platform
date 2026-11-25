import { Navigate } from 'react-router-dom'
import Monitoring from '../../components/Monitoring/Monitoring'
import Header from '../../components/Header/Header'
import TaskCarousel from '../../components/TaskCarousel/TaskCarousel'
import { useContest } from '../../state/contest'
import styles from './Tasks.module.css'

export default function Tasks() {
  const { participant, tasks, token, selectedTaskId } = useContest()

  if (!participant) return <Navigate to="/register" replace />

  return (
    <div className={styles.page}>
      <Header />
      {token && <Monitoring key={token} token={token} participantId={participant.id} taskId={selectedTaskId} />}
      <main className={styles.main}>
        {tasks.length === 0 ? <p>Загрузка задач…</p> : <TaskCarousel tasks={tasks} />}
      </main>
    </div>
  )
}
