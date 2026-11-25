import { Navigate, Route, Routes } from 'react-router-dom'
import { ContestProvider, useContest } from './state/contest'
import Register from './pages/Register/Register'
import Tasks from './pages/Tasks/Tasks'
import Leaderboard from './pages/Leaderboard/Leaderboard'
import ClockAdmin from './pages/ClockAdmin/ClockAdmin'
import DraftHistory from './pages/DraftHistory/DraftHistory'

function Root() {
  const { participant } = useContest()
  return <Navigate to={participant ? '/tasks' : '/register'} replace />
}

export default function App() {
  return (
    <ContestProvider>
      <Routes>
        <Route path="/" element={<Root />} />
        <Route path="/register" element={<Register />} />
        <Route path="/tasks" element={<Tasks />} />
        <Route path="/leaderboard" element={<Leaderboard />} />
        <Route path="/clock" element={<ClockAdmin />} />
        <Route path="/history" element={<DraftHistory />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </ContestProvider>
  )
}
