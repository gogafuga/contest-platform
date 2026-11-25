import { useEffect, useState } from 'react'
import type { TaskSpec } from '../../lib/api'
import { useContest } from '../../state/contest'
import CaseSlide from './CaseSlide'
import styles from './TaskCarousel.module.css'

interface Props { tasks: TaskSpec[] }

export default function TaskCarousel({ tasks }: Props) {
  const [active, setActive] = useState(0)
  const { selectedTaskId, isSolved } = useContest()
  useEffect(() => {
    const index = tasks.findIndex(task => task.id === selectedTaskId)
    if (index >= 0) setActive(index)
  }, [selectedTaskId, tasks])

  return <div className={styles.wrap}>
    <nav className={styles.tabs} aria-label="Задачи соревнования">
      {tasks.map((task, index) => <button key={task.id} type="button"
        className={[styles.tab, index === active ? styles.active : '', isSolved(task.id) ? styles.solved : ''].join(' ')}
        aria-pressed={index === active} onClick={() => setActive(index)}>
        <span>{task.code}</span> {task.title}
        {isSolved(task.id) && <span aria-label="Решено">✓</span>}
      </button>)}
    </nav>
    <div className={styles.workspace}>
      {tasks.map((task, index) => <div key={task.id} className={styles.panel} hidden={index !== active}>
        <CaseSlide task={task} focused={index === active} />
      </div>)}
    </div>
  </div>
}
