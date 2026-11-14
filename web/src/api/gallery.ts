import { clearFavoriteCache } from '../utils/favoriteCache'

export interface MediaItem {
  id: number
  name: string
  favorite: boolean
  media_type: 'image' | 'video'
  mime_type: string
  width_px?: number
  height_px?: number
  duration_ms?: number
  size_bytes: number
  thumbnail_url: string
  original_url: string
}

export interface AlbumItem {
  published_date: string
  id: number
  name: string
  favorite: boolean
  people: AlbumPersonItem[]
  series: AlbumSeriesItem[]
  media_count: number
  media: MediaItem[]
}

export interface AlbumSeriesItem {
  id: number
  name: string
}

export interface AlbumPersonItem {
  id: number
  name: string
  note: string
}

export interface Pagination {
  offset: number
  limit: number
  total: number
  has_more: boolean
}

export interface GalleryResponse {
  albums: AlbumItem[]
  total_media: number
  pagination: Pagination
}

export interface FavoritePhotosResponse {
  media: MediaItem[]
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
  return response.json() as Promise<T>
}

export function fetchGallery(offset = 0, limit = 10, favoritesOnly = false, search = '') {
  const query = new URLSearchParams({
    offset: String(offset),
    limit: String(limit),
  })
  if (favoritesOnly) query.set('favorites', 'true')
  if (search.trim()) query.set('q', search.trim())
  return request<GalleryResponse>(`/api/gallery?${query}`)
}

export async function updateAlbumFavorite(albumID: number, favorite: boolean) {
  const result = await request<{ id: number; favorite: boolean }>(`/api/albums/${albumID}/favorite`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ favorite }),
  })
  await clearFavoriteCache({ media: !favorite })
  return result
}

export function fetchFavoritePhotos(offset = 0, limit = 500) {
  const query = new URLSearchParams({ offset: String(offset), limit: String(limit) })
  return request<FavoritePhotosResponse>(`/api/favorites/photos?${query}`)
}

export async function updatePhotoFavorite(photoID: number, favorite: boolean) {
  const result = await request<{ id: number; favorite: boolean }>(`/api/media/${photoID}/favorite`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ favorite }),
  })
  await clearFavoriteCache({ photoID: favorite ? undefined : photoID })
  return result
}
