const cacheName = 'airy-favorites-api-v1'

async function cachedFavorites(url: string): Promise<Response | undefined> {
  if (!('caches' in globalThis)) return undefined
  try {
    const cache = await caches.open(cacheName)
    const response = await cache.match(url)
    if (!response) return undefined
    const date = Date.parse(response.headers.get('date') ?? '')
    if (Number.isFinite(date) && Date.now() - date > 7 * 24 * 60 * 60 * 1000) return undefined
    return response
  } catch { return undefined }
}

// Read the same cache as the service worker without waiting on an offline network.
export async function fetchOfflineFavorites(url: string): Promise<Response> {
  if (!navigator.onLine) {
    const cached = await cachedFavorites(url)
    if (cached) return cached
    throw new Error('此收藏列表尚未缓存，请联网后打开收藏页。')
  }
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 8000)
  try {
    return await fetch(url, { headers: { Accept: 'application/json' }, signal: controller.signal })
  } catch (error) {
    const cached = await cachedFavorites(url)
    if (cached) return cached
    throw error
  } finally { clearTimeout(timeout) }
}
