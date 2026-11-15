// Storage can be unavailable in restricted browser contexts. Preferences must not block startup.
export const safeStorage = {
  getItem(key: string): string | null {
    try { return localStorage.getItem(key) } catch { return null }
  },
  setItem(key: string, value: string) {
    try { localStorage.setItem(key, value) } catch { /* Keep in-memory preferences. */ }
  },
  removeItem(key: string) {
    try { localStorage.removeItem(key) } catch { /* Storage is unavailable. */ }
  },
}
