const storageKey = 'airy-offline-session-v1'
const maxAge = 30 * 24 * 60 * 60 * 1000

export function rememberOfflineSession(username: string) {
  try { localStorage.setItem(storageKey, JSON.stringify({ username, savedAt: Date.now() })) } catch { /* Private storage may be unavailable. */ }
}

export function clearOfflineSession() {
  try { localStorage.removeItem(storageKey) } catch { /* Private storage may be unavailable. */ }
}

export function readOfflineSession(): { username: string } | null {
  try {
    const value = JSON.parse(localStorage.getItem(storageKey) ?? 'null')
    const age = Date.now() - value?.savedAt
    if (typeof value?.username !== 'string' || !Number.isFinite(age) || age < 0 || age > maxAge) return null
    return { username: value.username }
  } catch { return null }
}
