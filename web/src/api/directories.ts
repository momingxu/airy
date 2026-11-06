export interface ScanStats {
  albums: number
  files: number
  images: number
  videos: number
  skipped_files: number
  metadata_errors: number
  derivative_errors: number
  thumbnails_ready: number
}

export interface ScanStatus {
  state: 'idle' | 'scanning' | 'completed' | 'failed' | 'canceled'
  stage?: 'discovering_albums' | 'scanning_media' | 'generating_thumbnails'
  root?: string
  queued: number
  started_at?: string
  finished_at?: string
  stats: ScanStats
  error?: string
  canceling: boolean
}

export interface ScanSchedule {
  enabled: boolean
  scan_time: string
  timezone: string
  next_scan_at?: string
}

export interface ResourceDirectory {
  id: number
  name: string
  path: string
  album_depth: number
  scan_status: boolean
  last_scanned_at?: string
  scan_error?: string
  automation_disabled: boolean
  watching: boolean
  watch_error?: string
  album_count: number
  media_count: number
  scanning: boolean
  queued: boolean
}

export interface DirectoriesResponse {
  directories: ResourceDirectory[]
  scan: ScanStatus
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: { Accept: 'application/json', ...init?.headers },
  })
  if (response.status === 401) window.dispatchEvent(new Event('airy:unauthorized'))
  if (!response.ok) {
    const body = await response.json().catch(() => null)
    throw new Error(body?.error ?? `请求失败（${response.status}）`)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export function fetchDirectories() {
  return request<DirectoriesResponse>('/api/directories')
}

export function createDirectory(path: string, albumDepth = 1) {
  return request<{ directory: ResourceDirectory }>('/api/directories', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, album_depth: albumDepth }),
  })
}

export function scanDirectory(id: number) {
  return request<{ scan: ScanStatus }>(`/api/directories/${id}/scan`, { method: 'POST' })
}

export function scanAllDirectories() {
  return request<{ queued: number; scan: ScanStatus }>('/api/scan/all', { method: 'POST' })
}

export function cancelScan() {
  return request<{ scan: ScanStatus }>('/api/scan/cancel', { method: 'POST' })
}

export function fetchScanSchedule() {
  return request<{ schedule: ScanSchedule }>('/api/scan/schedule')
}

export function updateScanSchedule(settings: Pick<ScanSchedule, 'enabled'>) {
  return request<{ schedule: ScanSchedule }>('/api/scan/schedule', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  })
}

export function updateDirectorySettings(
  id: number,
  settings: Pick<ResourceDirectory, 'automation_disabled'>,
) {
  return request<{ directory: ResourceDirectory }>(`/api/directories/${id}/settings`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  })
}

export function deleteDirectory(id: number) {
  return request<void>(`/api/directories/${id}`, { method: 'DELETE' })
}
