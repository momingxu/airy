import type { AlbumItem, Pagination } from './gallery'
import type { AlbumSearchItem } from './people'

export interface SeriesItem {
  id: number
  name: string
  favorite: boolean
  cover_url: string
  album_count: number
  created_at: string
}

export interface SeriesGalleryResponse {
  series: SeriesItem
  albums: AlbumItem[]
  total_media: number
  pagination: Pagination
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

export function fetchSeries() {
  return request<{ series: SeriesItem[] }>('/api/series')
}

export function createSeries(name: string, albumIDs: number[] = []) {
  return request<{ series: SeriesItem }>('/api/series', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, album_ids: albumIDs }),
  })
}

export function updateSeries(id: number, changes: { name?: string; favorite?: boolean; album_ids?: number[]; remove_album_ids?: number[] }) {
  return request<{ series: SeriesItem }>(`/api/series/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(changes),
  })
}

export function deleteSeries(id: number) {
  return request<void>(`/api/series/${id}`, { method: 'DELETE' })
}

export function fetchSeriesGallery(seriesID: number, offset = 0, limit = 10) {
  const query = new URLSearchParams({
    offset: String(offset),
    limit: String(limit),
  })
  return request<SeriesGalleryResponse>(`/api/series/${seriesID}/gallery?${query}`)
}

export function fetchSeriesAlbumOptions(seriesID: number, search = '', regex = false) {
  const query = new URLSearchParams({ q: search, regex: String(regex) })
  return request<{ albums: Array<AlbumSearchItem & { associated: boolean }> }>(
    `/api/series/${seriesID}/album-options?${query}`,
  )
}

export function addSeriesAlbum(seriesID: number, albumID: number) {
  return request<{ series: SeriesItem }>(`/api/series/${seriesID}/albums/${albumID}`, { method: 'PUT' })
}

export function removeSeriesAlbum(seriesID: number, albumID: number) {
  return request<{ series: SeriesItem }>(`/api/series/${seriesID}/albums/${albumID}`, { method: 'DELETE' })
}
