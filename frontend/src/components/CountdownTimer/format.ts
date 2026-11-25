export function formatCountdown(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds))
  const days = Math.floor(total / 86400)
  const hours = Math.floor((total % 86400) / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const pad = (value: number) => value.toString().padStart(2, '0')

  if (days > 0) {
    return `${pad(days)}:${pad(hours)}:${pad(minutes)}:${pad(total % 60)}`
  }
  if (total >= 3600) {
    return `${pad(hours)}:${pad(minutes)}:${pad(total % 60)}`
  }
  return `${pad(minutes)}:${pad(total % 60)}`
}
