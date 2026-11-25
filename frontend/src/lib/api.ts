import type { EventType, PendingEvent } from './eventQueue'
export type { EventType, PendingEvent } from './eventQueue'
import type { Language } from './languages'
const API_URL = import.meta.env.VITE_API_URL || ''

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function failWith(res: Response): Promise<never> {
  let message = `Ошибка запроса (${res.status})`
  try {
    const data = await res.json()
    if (typeof data?.error === 'string') message = data.error
  } catch {
    // body wasn't JSON — keep the generic message
  }
  throw new ApiError(res.status, message)
}

export interface Participant {
  id: number
  name: string
  group: string
}

export interface TaskSpec {
  id: string
  code: string
  title: string
  points: number
  brief: string
  rules: string[]
  sampleInput: string
  sampleOutput: string
}

export interface ClockState {
  startedAt: string | null
  durationSeconds: number
  status: 'not_started' | 'running' | 'ended'
}

export interface LeaderboardEntry {
  name: string
  group: string
  scores: Record<string, number>
  total: number
}

export interface LeaderboardResponse {
  tasks: TaskSpec[]
  entries: LeaderboardEntry[]
}

export interface SubmissionResult {
  taskId: string
  passed: boolean
  feedback: string
  earned: number
}

export interface MeResponse {
  participant: Participant
  selectedTaskId: string | null
  scores: Record<string, number>
}

export async function register(
  name: string,
  group: string,
): Promise<{ token: string; participant: Participant }> {
  const res = await fetch(`${API_URL}/api/v1/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, group }),
  })
  if (!res.ok) return failWith(res)
  return res.json()
}

export async function getClock(): Promise<ClockState> {
  const res = await fetch(`${API_URL}/api/v1/clock`)
  if (!res.ok) return failWith(res)
  return res.json()
}

export async function startClock(adminToken: string, durationSeconds?: number): Promise<ClockState> {
  const res = await fetch(`${API_URL}/api/v1/admin/clock/start`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${adminToken}`,
      ...(durationSeconds !== undefined ? { 'Content-Type': 'application/json' } : {}),
    },
    body: durationSeconds !== undefined ? JSON.stringify({ durationSeconds }) : undefined,
  })
  if (!res.ok) return failWith(res)
  return res.json()
}

export async function getLeaderboard(): Promise<LeaderboardResponse> {
  const res = await fetch(`${API_URL}/api/v1/leaderboard`)
  if (!res.ok) return failWith(res)
  return res.json()
}

export async function getTasks(): Promise<TaskSpec[]> {
  const res = await fetch(`${API_URL}/api/v1/tasks`)
  if (!res.ok) return failWith(res)
  return res.json()
}

export async function getTaskInput(taskId: string, token: string): Promise<Blob> {
  const res = await fetch(`${API_URL}/api/v1/tasks/${taskId}/input`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) return failWith(res)
  return res.blob()
}

export async function getMe(token: string): Promise<MeResponse> {
  const res = await fetch(`${API_URL}/api/v1/me`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) return failWith(res)
  return res.json()
}

export async function selectTask(
  taskId: string,
  token: string,
): Promise<{ selectedTaskId: string | null }> {
  const res = await fetch(`${API_URL}/api/v1/tasks/${taskId}/select`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) return failWith(res)
  return res.json()
}

export async function submitTask(
  taskId: string,
  token: string,
  file: File,
): Promise<SubmissionResult> {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch(`${API_URL}/api/v1/tasks/${taskId}/submit`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  })
  if (!res.ok) return failWith(res)
  return res.json()
}

export interface Draft {
  language: Language
  sourceCode: string
  version: number
  updatedAt: string | null
}

export async function getDraft(taskId: string, token: string): Promise<Draft> {
  const res = await fetch(`${API_URL}/api/v1/tasks/${taskId}/draft`, {
    signal: AbortSignal.timeout(15000),
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) return failWith(res)
  return res.json()
}

export async function saveDraft(
  taskId: string, token: string, sourceCode: string, version: number, language: Language,
): Promise<Draft> {
  const res = await fetch(`${API_URL}/api/v1/tasks/${taskId}/draft`, {
    method: 'PUT',
    signal: AbortSignal.timeout(15000),
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ sourceCode, version, language }),
  })
  if (!res.ok) return failWith(res)
  return res.json()
}

export interface HistoryEntry {
  version: number
  language: Language
  createdAt: string
}
export interface HistorySnapshot extends HistoryEntry {
  sourceCode: string
}
export async function getDraftHistory(participantId: number, taskId: string, token: string): Promise<HistoryEntry[]> {
  const res = await fetch(`${API_URL}/api/v1/admin/participants/${participantId}/tasks/${encodeURIComponent(taskId)}/history`, {
    headers: { Authorization: `Bearer ${token}` }, signal: AbortSignal.timeout(15000),
  })
  if (!res.ok) return failWith(res)
  return res.json()
}
export async function getDraftHistoryVersion(participantId: number, taskId: string, version: number, token: string): Promise<HistorySnapshot> {
  const res = await fetch(`${API_URL}/api/v1/admin/participants/${participantId}/tasks/${encodeURIComponent(taskId)}/history/${version}`, {
    headers: { Authorization: `Bearer ${token}` }, signal: AbortSignal.timeout(15000),
  })
  if (!res.ok) return failWith(res)
  return res.json()
}

export interface ParticipantEvent {
  id: number
  eventType: EventType
  clientAt: string
  receivedAt: string
}
export interface TaskTiming {
  startedAt: string | null
  finishedAt: string | null
  elapsedSeconds: number
  estimated: boolean
}
export async function getTaskTiming(participantId: number, taskId: string, token: string, signal?: AbortSignal): Promise<TaskTiming> {
  const res = await fetch(`${API_URL}/api/v1/admin/participants/${participantId}/tasks/${encodeURIComponent(taskId)}/timing`, {
    headers: { Authorization: `Bearer ${token}` },
    signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(15000)]) : AbortSignal.timeout(15000),
  })
  if (!res.ok) return failWith(res)
  return res.json()
}
export async function sendParticipantEvent(event: PendingEvent, token: string): Promise<Response> {
  return fetch(`${API_URL}/api/v1/tasks/${encodeURIComponent(event.taskId)}/events`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ eventId: event.eventId, eventType: event.eventType, clientAt: event.clientAt }),
    signal: AbortSignal.timeout(10000),
    keepalive: true,
  })
}
export async function getParticipantEvents(participantId: number, taskId: string, afterId: number, token: string): Promise<ParticipantEvent[]> {
  const res = await fetch(`${API_URL}/api/v1/admin/participants/${participantId}/tasks/${encodeURIComponent(taskId)}/events?afterId=${afterId}`, {
    headers: { Authorization: `Bearer ${token}` }, signal: AbortSignal.timeout(15000),
  })
  if (!res.ok) return failWith(res)
  return res.json()
}

export async function searchParticipants(query: string, token: string, signal?: AbortSignal): Promise<Participant[]> {
  const res = await fetch(`${API_URL}/api/v1/admin/participants?q=${encodeURIComponent(query)}`, {
    headers: { Authorization: `Bearer ${token}` },
    signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(10000)]) : AbortSignal.timeout(10000),
  })
  if (!res.ok) return failWith(res)
  return res.json()
}

export interface CodeRun {
  id: string
  language: Language
  status: string
  stdout: string
  stderr: string
  outputTruncated: boolean
  exitCode: number
  durationMs: number
  passed: boolean
  earned: number
  feedback: string
  createdAt: string
}
export async function createCodeRun(taskId: string, token: string, requestId: string, language: Language, sourceCode: string): Promise<CodeRun> {
  const response = await fetch(`${API_URL}/api/v1/tasks/${encodeURIComponent(taskId)}/runs`, {
    method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ requestId, language, sourceCode }), signal: AbortSignal.timeout(15000),
  })
  if (!response.ok) return failWith(response)
  return response.json()
}
export async function getCodeRun(taskId: string, token: string, id: string): Promise<CodeRun> {
  const response = await fetch(`${API_URL}/api/v1/tasks/${encodeURIComponent(taskId)}/runs/${encodeURIComponent(id)}`, {
    headers: { Authorization: `Bearer ${token}` }, signal: AbortSignal.timeout(15000),
  })
  if (!response.ok) return failWith(response)
  return response.json()
}
