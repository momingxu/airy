import { clearOfflineSession, readOfflineSession, rememberOfflineSession } from '../utils/offlineSession'

export interface AuthStatus {
  configured: boolean
  authenticated: boolean
  username?: string
  offline?: boolean
}

interface UserResponse {
  user: { username: string }
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: { Accept: 'application/json', ...init?.headers },
  })
  if (!response.ok) {
    const body = await response.json().catch(() => null)
    throw new Error(body?.error ?? `请求失败（${response.status}）`)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export async function fetchAuthStatus(): Promise<AuthStatus> {
  let response: Response
  try {
    response = await fetch('/api/auth/status', { headers: { Accept: 'application/json' }, cache: 'no-store' })
  } catch (error) {
    // Only network failure permits local browsing. A server rejection must win.
    const saved = readOfflineSession()
    if (saved) return { configured: true, authenticated: true, username: saved.username, offline: true }
    throw error
  }
  if (!response.ok) {
    if (response.status === 401 || response.status === 403) clearOfflineSession()
    throw new Error(`无法读取登录状态（${response.status}）`)
  }
  const status = await response.json() as AuthStatus
  if (status.configured && status.authenticated) rememberOfflineSession(status.username ?? '')
  else clearOfflineSession()
  return status
}

export function setupAccount(username: string, password: string) {
  return request<UserResponse>('/api/auth/setup', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
}

export function login(username: string, password: string) {
  return request<UserResponse>('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
}

export async function logout() {
  try { return await request<void>('/api/auth/logout', { method: 'POST' }) }
  finally { clearOfflineSession() }
}

export function updateAccount(username: string, currentPassword: string, newPassword: string) {
  return request<UserResponse>('/api/settings/account', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      username,
      current_password: currentPassword,
      new_password: newPassword,
    }),
  })
}
