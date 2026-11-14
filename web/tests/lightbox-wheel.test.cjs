const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const assert = require('node:assert/strict')
const { test } = require('node:test')
const ts = require('typescript')

// Execute the actual handlers with a virtual clock and controlled image decoding.
const source = fs.readFileSync(path.join(__dirname, '../src/App.vue'), 'utf8')
const handlers = [
  ['async function stepPhoto(', 'function enterOfflineMode('],
  ['function handleLightboxWheel(', 'function closeFromBackdrop('],
].map(([start, end]) => source.slice(source.indexOf(start), source.indexOf(end, source.indexOf(start)))).join('\n')
const script = ts.transpileModule(handlers, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText

function setup(decodeDelay = 0) {
  let now = 0
  let ready = !decodeDelay
  let nextTimer = 1
  const tasks = []
  const changes = []
  const later = (delay, fn) => {
    const id = nextTimer++
    tasks.push({ time: now + delay, fn, id })
    return id
  }
  const index = {
    index: 0,
    get value() { return this.index },
    set value(value) { this.index = value; changes.push({ time: now, index: value }) },
  }
  const context = {
    lightboxClosing: false,
    previewPhotos: { value: Array.from({ length: 8 }, (_, id) => ({ id: String(id), mediaType: 'image', original: `/image/${id}` })) },
    previewIndex: index, lightbox: { value: { open: true } },
    lightboxSwitchPending: false, queuedLightboxDirection: 0, lightboxSwitchGeneration: 0,
    lightboxLoadingPhotoID: { value: null }, lightboxZoomed: { value: false },
    wheelZoomModifier: { value: 'ctrl' }, modifierShortcutChorded: false,
    matchesWheelZoom: event => event.ctrlKey === true,
    preloadedLightboxImages: new Map(), performance: { now: () => now }, wheelLastSwitchAt: -Infinity,
    WheelEvent: { DOM_DELTA_LINE: 1, DOM_DELTA_PAGE: 2 },
    window: { innerHeight: 800, setTimeout: (fn, delay) => later(delay, fn), clearTimeout: id => {
      const index = tasks.findIndex(task => task.id === id)
      if (index >= 0) tasks.splice(index, 1)
    } },
    document: { hidden: false }, slideshowTimer: undefined,
    slideshowPlaying: { value: false }, currentPhoto: { value: { mediaType: 'image' } },
    slideshowProgressKey: { value: 0 }, slideshowDelay: 5000,
    nextTick: async () => {},
    Image: class { decode() { return new Promise(resolve => later(decodeDelay, () => { ready = true; resolve() })) } },
    lightboxSlides: { value: {
      isPhotoReady: () => ready,
      slide: () => new Promise(resolve => later(320, resolve)),
      resetNavigation() {}, zoomWithWheel() { context.zoomCalls++ },
    } }, zoomCalls: 0,
  }
  vm.createContext(context)
  vm.runInContext(script, context)
  return {
    context, changes,
    wheelAt(time, extra = {}) { tasks.push({ time, id: nextTimer++, fn: () => context.handleLightboxWheel({ deltaY: 100, deltaX: 0, deltaMode: 0, ...extra }) }) },
    async advance(until) {
      while (tasks.some(task => task.time <= until)) {
        tasks.sort((a, b) => a.time - b.time)
        const task = tasks.shift()
        now = task.time
        task.fn()
        await new Promise(resolve => setImmediate(resolve))
      }
      now = until
    },
  }
}

test('slow decoding does not queue wheel input or bypass cooldown after completion', async () => {
  const state = setup(900)
  for (const time of [0, 500, 1000, 1500, 1719, 1720]) state.wheelAt(time)
  await state.advance(2040)
  assert.deepEqual(state.changes, [{ time: 1220, index: 1 }, { time: 2040, index: 2 }])
  assert.equal(state.context.queuedLightboxDirection, 0)
})

test('cached images still require 500ms after animation before another wheel switch', async () => {
  const state = setup()
  for (const time of [0, 100, 319, 500, 819, 820]) state.wheelAt(time)
  await state.advance(1140)
  assert.deepEqual(state.changes, [{ time: 320, index: 1 }, { time: 1140, index: 2 }])
})

test('modifier wheel continues to zoom instead of entering slide cooldown', async () => {
  const state = setup()
  state.wheelAt(0)
  state.wheelAt(100, { ctrlKey: true })
  await state.advance(400)
  assert.equal(state.context.zoomCalls, 1)
  assert.equal(state.changes.length, 1)
})

test('autoplay waits until manual decoding and animation have completed', async () => {
  const state = setup(6000)
  state.context.slideshowPlaying.value = true
  state.context.scheduleSlideshow()
  state.wheelAt(100)
  await state.advance(7000)
  assert.deepEqual(state.changes, [{ time: 6420, index: 1 }])
  assert.equal(state.context.queuedLightboxDirection, 0)
})
