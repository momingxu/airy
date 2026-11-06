import type { AlbumItem, Pagination } from './gallery'

export interface PersonItem {
  id: number
  name: string
  note: string
  aliases: string[]
  favorite: boolean
  avatar_url: string
  album_count: number
  created_at: string
}

export interface PersonAlbumOption {
  id: number
  name: string
  associated: boolean
}

export interface AlbumSearchItem {
  id: number
  name: string
}

export interface PersonGalleryResponse {
  person: PersonItem
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

export function fetchPeople() {
  return request<{ people: PersonItem[] }>('/api/people')
}

export function createPerson(
  name: string,
  albumIDs: number[] = [],
  note = '',
  aliases: string[] = [],
) {
  return request<{ person: PersonItem }>('/api/people', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, note, aliases, album_ids: albumIDs }),
  })
}

export function updatePerson(
  id: number,
  changes: { name?: string; note?: string; aliases?: string[]; album_ids?: number[]; remove_album_ids?: number[] },
) {
  return request<{ person: PersonItem }>(`/api/people/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(changes),
  })
}

export function searchAlbums(search: string) {
  const query = new URLSearchParams({ q: search })
  return request<{ albums: AlbumSearchItem[] }>(`/api/albums/search?${query}`)
}

export function deletePerson(id: number) {
  return request<void>(`/api/people/${id}`, { method: 'DELETE' })
}

export function updatePersonFavorite(id: number, favorite: boolean) {
  return request<{ id: number; favorite: boolean }>(`/api/people/${id}/favorite`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ favorite }),
  })
}

export function fetchPersonGallery(personID: number, offset = 0, limit = 10) {
  const query = new URLSearchParams({
    offset: String(offset),
    limit: String(limit),
  })
  return request<PersonGalleryResponse>(`/api/people/${personID}/gallery?${query}`)
}

export function fetchPersonAlbumOptions(personID: number, search = '') {
  const query = new URLSearchParams({ q: search })
  return request<{ albums: PersonAlbumOption[] }>(`/api/people/${personID}/album-options?${query}`)
}

export function addPersonAlbum(personID: number, albumID: number) {
  return request<{ person: PersonItem }>(`/api/people/${personID}/albums/${albumID}`, { method: 'PUT' })
}

export function removePersonAlbum(personID: number, albumID: number) {
  return request<{ person: PersonItem }>(`/api/people/${personID}/albums/${albumID}`, { method: 'DELETE' })
}
