import type { Photo } from '../types/gallery'

const cachePrefix = 'airy-favorites-'

function mediaURL(path: string) {
  const [base, query = ''] = path.split('?')
  const params = new URLSearchParams(query)
  params.set('favorite_cache', '1')
  return `${base}?${params}`
}

export function favoritePhoto(photo: Photo): Photo {
  return {
    ...photo,
    thumbnail: mediaURL(photo.thumbnail),
    original: photo.mediaType === 'image' ? mediaURL(photo.original) : photo.original,
  }
}

// Cache failures must never make login, logout or a successful favorite edit fail.
export async function clearFavoriteCache(options: { media?: boolean; photoID?: number } = {}) {
  if (!('caches' in window)) return
  try {
    const names = (await caches.keys()).filter(name => name.startsWith(cachePrefix))
    for (const name of names) {
      if (options.media || name === `${cachePrefix}api-v1`) {
        await caches.delete(name)
      } else if (options.photoID !== undefined) {
        const cache = await caches.open(name)
        const entries = await cache.keys()
        for (const entry of entries) {
          if (new URL(entry.url).pathname.startsWith(`/api/media/${options.photoID}/`)) await cache.delete(entry)
        }
      }
    }
  } catch { /* Storage may be unavailable or full. */ }
}
