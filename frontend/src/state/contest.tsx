import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import {
  ApiError,
  getMe,
  getTasks,
  selectTask as apiSelectTask,
  type Participant,
  type TaskSpec,
} from '../lib/api'

interface ContestState {
  participant: Participant | null
  token: string | null
  tasks: TaskSpec[]
  scores: Record<string, number>
  selectedTaskId: string | null
  register: (token: string, participant: Participant) => void
  setScore: (taskId: string, score: number) => void
  selectTask: (taskId: string) => Promise<void>
  isSolved: (taskId: string) => boolean
  total: number
}

const ContestContext = createContext<ContestState | null>(null)

const TOKEN_KEY = 'devsec_token'
const PARTICIPANT_KEY = 'devsec_participant'
const SCORES_KEY = 'devsec_scores'
const SELECTED_TASK_KEY = 'devsec_selected_task'

export function ContestProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() => localStorage.getItem(TOKEN_KEY))
  const [participant, setParticipant] = useState<Participant | null>(() => {
    const raw = localStorage.getItem(PARTICIPANT_KEY)
    return raw ? (JSON.parse(raw) as Participant) : null
  })
  const [scores, setScores] = useState<Record<string, number>>(() => {
    const raw = localStorage.getItem(SCORES_KEY)
    return raw ? (JSON.parse(raw) as Record<string, number>) : {}
  })
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(() =>
    localStorage.getItem(SELECTED_TASK_KEY),
  )
  const [tasks, setTasks] = useState<TaskSpec[]>([])

  useEffect(() => {
    getTasks()
      .then(setTasks)
      .catch(() => {
        // transient network hiccup — the page keeps rendering a loading state
      })
  }, [])

  useEffect(() => {
    if (token) localStorage.setItem(TOKEN_KEY, token)
  }, [token])

  useEffect(() => {
    if (participant) localStorage.setItem(PARTICIPANT_KEY, JSON.stringify(participant))
  }, [participant])

  useEffect(() => {
    localStorage.setItem(SCORES_KEY, JSON.stringify(scores))
  }, [scores])

  useEffect(() => {
    if (selectedTaskId) localStorage.setItem(SELECTED_TASK_KEY, selectedTaskId)
    else localStorage.removeItem(SELECTED_TASK_KEY)
  }, [selectedTaskId])

  // Backend is the source of truth for selection/scores — resync on load (or
  // after a fresh register) so a stale/cleared local cache can't desync from
  // what the server actually has on record.
  useEffect(() => {
    if (!token) return
    getMe(token)
      .then((me) => {
        setScores(me.scores)
        setSelectedTaskId(me.selectedTaskId)
      })
      .catch((err) => {
        if (err instanceof ApiError && err.status === 401) {
          setToken(null)
          setParticipant(null)
          setScores({})
          setSelectedTaskId(null)
          localStorage.removeItem(TOKEN_KEY)
          localStorage.removeItem(PARTICIPANT_KEY)
          localStorage.removeItem(SCORES_KEY)
          localStorage.removeItem(SELECTED_TASK_KEY)
        }
      })
  }, [token])

  const register = (newToken: string, p: Participant) => {
    setToken(newToken)
    setParticipant(p)
    setScores({})
    setSelectedTaskId(null)
  }

  const setScore = (taskId: string, score: number) => {
    setScores((prev) => ({ ...prev, [taskId]: score }))
  }

  const isSolved = (taskId: string) => {
    const task = tasks.find((t) => t.id === taskId)
    return task ? (scores[taskId] ?? 0) >= task.points : false
  }

  const selectTask = async (taskId: string) => {
    if (!token || selectedTaskId === taskId) return
    const res = await apiSelectTask(taskId, token)
    setSelectedTaskId(res.selectedTaskId)
  }

  const total = useMemo(() => Object.values(scores).reduce((a, b) => a + b, 0), [scores])

  return (
    <ContestContext.Provider
      value={{
        participant,
        token,
        tasks,
        scores,
        selectedTaskId,
        register,
        setScore,
        selectTask,
        isSolved,
        total,
      }}
    >
      {children}
    </ContestContext.Provider>
  )
}

export function useContest() {
  const ctx = useContext(ContestContext)
  if (!ctx) throw new Error('useContest must be used within ContestProvider')
  return ctx
}
