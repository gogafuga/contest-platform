// The suggestion widget must not wait for a cold sandbox. The caller keeps
// the shared request alive and refreshes the widget when semantic results arrive.
export async function withinCompletionBudget<T>(work: Promise<T>, fallback: T, milliseconds = 150): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined
  try {
    return await Promise.race([work, new Promise<T>(resolve => {
      timer = setTimeout(() => resolve(fallback), milliseconds)
    })])
  } finally {
    clearTimeout(timer)
  }
}
