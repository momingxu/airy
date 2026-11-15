const { test } = require('node:test')
const assert = require('node:assert/strict')
const { readFileSync } = require('node:fs')
const vm = require('node:vm')
const ts = require('typescript')

function setup(fetch, saved = { username: 'owner' }) {
  let expire
  let cleared = false
  let remembered
  const module = { exports: {} }
  const source = ts.transpileModule(readFileSync(`${__dirname}/../src/api/auth.ts`, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
  }).outputText
  vm.runInNewContext(source, {
    exports: module.exports, module, AbortController, fetch,
    setTimeout(callback, delay) { assert.equal(delay, 5000); expire = callback; return 1 },
    clearTimeout() {},
    require() { return {
      readOfflineSession: () => saved,
      clearOfflineSession: () => { cleared = true },
      rememberOfflineSession: username => { remembered = username },
    } },
  })
  return { auth: module.exports, expire: () => expire(), cleared: () => cleared, remembered: () => remembered }
}

test('a stalled connection aborts and opens the saved offline session', async () => {
  const app = setup((_url, { signal }) => new Promise((_, reject) => {
    signal.addEventListener('abort', () => reject(new Error('aborted')))
  }))
  const pending = app.auth.fetchAuthStatus()
  app.expire()
  const status = await pending
  assert.equal(status.offline, true)
  assert.equal(status.username, 'owner')
})

test('network failure without a saved session does not authenticate', async () => {
  const app = setup(async () => { throw new Error('offline') }, null)
  await assert.rejects(app.auth.fetchAuthStatus(), /offline/)
})

test('server rejection wins over a saved offline session', async () => {
  for (const status of [401, 403]) {
    const app = setup(async () => ({ ok: false, status }))
    await assert.rejects(app.auth.fetchAuthStatus())
    assert.equal(app.cleared(), true)
  }
})

test('an online logged-out response clears offline access', async () => {
  const app = setup(async () => ({ ok: true, json: async () => ({ configured: true, authenticated: false }) }))
  assert.equal((await app.auth.fetchAuthStatus()).authenticated, false)
  assert.equal(app.cleared(), true)
})

test('online login status refreshes the saved session', async () => {
  const app = setup(async () => ({ ok: true, json: async () => ({ configured: true, authenticated: true, username: 'owner' }) }))
  assert.equal((await app.auth.fetchAuthStatus()).authenticated, true)
  assert.equal(app.remembered(), 'owner')
})
