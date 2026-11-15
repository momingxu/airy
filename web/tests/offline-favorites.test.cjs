const { test } = require('node:test')
const assert = require('node:assert/strict')
const vm = require('node:vm')
const { readFileSync } = require('node:fs')
const ts = require('typescript')
function setup({ online = false, cached, fetch = () => { throw new Error('unexpected network') } } = {}) {
  const module = { exports: {} }
  let expire
  const source = ts.transpileModule(readFileSync(`${__dirname}/../src/utils/offlineFavorites.ts`, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
  }).outputText
  vm.runInNewContext(source, {
    module, exports: module.exports, navigator: { onLine: online }, fetch, AbortController,
    caches: { open: async name => { assert.equal(name, 'airy-favorites-api-v1'); return { match: async () => cached } } },
    setTimeout(callback) { expire = callback; return 1 }, clearTimeout() {},
  })
  return { read: module.exports.fetchOfflineFavorites, expire: () => expire() }
}
const cached = { headers: { get: () => new Date().toUTCString() } }
test('offline favorites use cache without a network request', async () => {
  assert.equal(await setup({ cached }).read('/api/favorites/photos?offset=0'), cached)
})
test('uncached offline lists show an actionable error', async () => {
  await assert.rejects(setup().read('/api/favorites/photos?offset=0'), /尚未缓存/)
})
test('stalled online requests fall back to cache after timeout', async () => {
  const app = setup({ online: true, cached, fetch: (_url, { signal }) => new Promise((_, reject) => {
    signal.addEventListener('abort', () => reject(new Error('timeout')))
  }) })
  const pending = app.read('/api/favorites/photos?offset=0')
  app.expire()
  assert.equal(await pending, cached)
})
test('server rejection is returned without a cache fallback', async () => {
  const response = { status: 401 }
  assert.equal(await setup({ online: true, cached, fetch: async () => response }).read('/api/favorites/photos?offset=0'), response)
})
test('expired cache is not returned offline', async () => {
  const expired = { headers: { get: () => new Date(Date.now() - 8 * 86400000).toUTCString() } }
  await assert.rejects(setup({ cached: expired }).read('/api/favorites/photos?offset=0'), /尚未缓存/)
})
