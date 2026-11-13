<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { matchesWheelZoom, type WheelZoomModifier } from '../utils/wheelZoom'
import DecodedPreviewImage from './DecodedPreviewImage.vue'
import type { Photo } from '../types/gallery'

const props = defineProps<{ photos: Photo[]; index: number; loading: boolean; wheelZoomModifier: WheelZoomModifier }>()
const emit = defineEmits<{
  navigate: [direction: number]
  dismiss: []
  loaded: [id: string]
  failed: [id: string]
  ended: []
  zoomChange: [zoomed: boolean]
  zoomGesture: []
}>()
const viewport = ref<HTMLDivElement | null>(null)
const track = ref<HTMLDivElement | null>(null)
const dragging = ref(false)
const pressed = ref(false)
const overImage = ref(false)
const moving = ref(false)
const zoom = ref(1)
const zoomed = ref(false)
let panX = 0
let panY = 0
let panStartX = 0
let panStartY = 0
const touchPointers = new Map<number, { x: number; y: number }>()
let pinch: { distance: number; scale: number; x: number; y: number; panX: number; panY: number } | null = null
let lastTouchTap: { time: number; x: number; y: number } | null = null
const slides = computed(() => {
  const offsets = props.photos.length < 2 ? [0] : [-1, 0, 1]
  return offsets.map(offset => {
    const index = (props.index + offset + props.photos.length) % props.photos.length
    return { offset, photo: props.photos[index], key: `${index}${props.photos.length === 2 && offset === -1 ? '-copy' : ''}` }
  })
})
let position = 0
let frame = 0
let hoverFrame = 0
let dragPitch = 0
let animation: Animation | null = null
let start: { x: number; y: number; time: number; pointerId: number; pointerType: string } | null = null
let horizontal = false
let awaitingNavigation = false
let suppressClickUntil = 0
let disposed = false
let closing = false
let closeAnimation: Animation | null = null

function paintZoom() {
  const image = track.value?.querySelector<HTMLImageElement>('.lightbox-slide[aria-hidden="false"] img')
  if (!image || !viewport.value) return
  const ratio = image.naturalWidth / image.naturalHeight
  const width = viewport.value.clientWidth
  const height = viewport.value.clientHeight
  const fitWidth = Math.min(width, height * ratio)
  const fitHeight = Math.min(height, width / ratio)
  const limitX = Math.max(0, (fitWidth * zoom.value - width) / 2)
  const limitY = Math.max(0, (fitHeight * zoom.value - height) / 2)
  panX = Math.max(-limitX, Math.min(limitX, panX))
  panY = Math.max(-limitY, Math.min(limitY, panY))
  image.style.transform = `translate3d(${panX}px,${panY}px,0) scale(${zoom.value})`
}
function toggleZoom() {
  const image = track.value?.querySelector<HTMLImageElement>('.lightbox-slide[aria-hidden="false"] img.ready')
  if (!image || !viewport.value || closing || awaitingNavigation) return
  // Switching modes must release the previous pan gesture and any queued frame.
  const pointerId = start?.pointerId
  start = null
  if (pointerId !== undefined) releasePointer(pointerId)
  cancelMotion()
  horizontal = false
  pressed.value = false
  dragging.value = false
  moving.value = false
  position = 0
  paint()
  zoomed.value = !zoomed.value
  zoom.value = zoomed.value ? 2 : 1
  panX = panY = panStartX = panStartY = 0
  paintZoom()
  emit('zoomChange', zoomed.value)
}
function zoomWithWheel(event: WheelEvent) {
  const image = track.value?.querySelector<HTMLImageElement>('.lightbox-slide[aria-hidden="false"] img.ready')
  if (!image || !viewport.value || closing || awaitingNavigation) return
  const pointerId = start?.pointerId
  start = null
  if (pointerId !== undefined) releasePointer(pointerId)
  cancelMotion()
  pressed.value = dragging.value = moving.value = false
  horizontal = false
  position = 0
  paint()
  const rect = viewport.value.getBoundingClientRect()
  const fitScale = Math.min(rect.width / image.naturalWidth, rect.height / image.naturalHeight)
  const rawDelta = Math.abs(event.deltaY) >= Math.abs(event.deltaX) ? event.deltaY : event.deltaX
  const delta = rawDelta * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? rect.height : 1)
  const next = Math.max(1, Math.min(Math.max(2, 2 / fitScale), zoom.value * Math.exp(-Math.max(-200, Math.min(200, delta)) * .002)))
  const x = event.clientX - rect.left - rect.width / 2
  const y = event.clientY - rect.top - rect.height / 2
  const ratio = next / zoom.value
  panX = x - (x - panX) * ratio
  panY = y - (y - panY) * ratio
  zoom.value = next
  zoomed.value = next > 1
  if (!zoomed.value) panX = panY = 0
  paintZoom()
  emit('zoomChange', zoomed.value)
}
function resetZoom() {
  zoomed.value = false
  zoom.value = 1
  panX = panY = 0
  track.value?.querySelectorAll<HTMLImageElement>('img').forEach(image => { image.style.transform = '' })
  emit('zoomChange', false)
}
function pitch() {
  const gap = viewport.value ? parseFloat(getComputedStyle(viewport.value).getPropertyValue('--lightbox-slide-gap')) : 12
  return (viewport.value?.clientWidth ?? 0) + gap
}
function paint() {
  if (track.value) track.value.style.transform = `translate3d(${position}px,0,0)`
}
function cancelMotion() {
  cancelAnimationFrame(frame)
  frame = 0
  animation?.cancel()
  animation = null
}
async function moveTo(target: number) {
  cancelMotion()
  const element = track.value
  if (!element || disposed) return
  moving.value = true
  const distance = Math.abs(target - position)
  const duration = matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : Math.min(320, Math.max(140, distance / pitch() * 320))
  animation = element.animate([
    { transform: `translate3d(${position}px,0,0)` },
    { transform: `translate3d(${target}px,0,0)` },
  ], { duration, easing: 'cubic-bezier(.22,.61,.36,1)', fill: 'forwards' })
  try { await animation.finished } catch { return }
  position = target
  paint()
  animation?.cancel()
  animation = null
  if (target === 0) moving.value = false
}
async function slide(direction: number) {
  awaitingNavigation = true
  if (start) releasePointer(start.pointerId)
  start = null
  dragging.value = false
  track.value?.querySelectorAll('video').forEach(video => video.pause())
  await moveTo(-direction * pitch())
}
async function fadeOut() {
  closing = true
  cancelMotion()
  const element = track.value
  if (!element) return
  closeAnimation = element.animate([{ opacity: 1 }, { opacity: 0 }], {
    duration: matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 140,
    easing: 'ease-out', fill: 'forwards',
  })
  try { await closeAnimation.finished } catch { /* Closing may unmount the preview. */ }
}
function isPhotoReady(id: string) {
  return [...(track.value?.querySelectorAll<HTMLImageElement>('img.ready') ?? [])].some(image => image.dataset.photoId === id)
}
defineExpose({ slide, fadeOut, resetNavigation, toggleZoom, zoomWithWheel, isPhotoReady })
function resetNavigation() {
  touchPointers.clear()
  pinch = null
  lastTouchTap = null
  const pointerId = start?.pointerId
  start = null
  if (pointerId !== undefined) releasePointer(pointerId)
  horizontal = false
  pressed.value = false
  dragging.value = false
  resetZoom()
  cancelMotion()
  position = 0
  moving.value = false
  awaitingNavigation = false
  paint()
}
watch(() => props.index, resetNavigation, { flush: 'post' })

function beginPinch() {
  const image = track.value?.querySelector<HTMLImageElement>('.lightbox-slide[aria-hidden="false"] img.ready')
  if (!image || !viewport.value || closing || awaitingNavigation) return false
  const [a, b] = [...touchPointers.values()]
  if (!a || !b) return false
  start = null
  cancelMotion()
  position = 0
  paint()
  horizontal = false
  moving.value = false
  dragging.value = pressed.value = true
  lastTouchTap = null
  const rect = viewport.value.getBoundingClientRect()
  pinch = { distance: Math.max(1, Math.hypot(b.x - a.x, b.y - a.y)), scale: zoom.value,
    x: (a.x + b.x) / 2 - rect.left - rect.width / 2,
    y: (a.y + b.y) / 2 - rect.top - rect.height / 2, panX, panY }
  for (const pointerId of touchPointers.keys()) viewport.value.setPointerCapture(pointerId)
  suppressClickUntil = performance.now() + 500
  return true
}
function movePinch(event: PointerEvent) {
  if (!pinch || !viewport.value || touchPointers.size < 2) return
  const image = track.value?.querySelector<HTMLImageElement>('.lightbox-slide[aria-hidden="false"] img.ready')
  if (!image) return
  const [a, b] = [...touchPointers.values()]
  const rect = viewport.value.getBoundingClientRect()
  const fit = Math.min(rect.width / image.naturalWidth, rect.height / image.naturalHeight)
  const scale = Math.max(1, Math.min(Math.max(2, 2 / fit), pinch.scale * Math.hypot(b.x - a.x, b.y - a.y) / pinch.distance))
  const ratio = scale / pinch.scale
  panX = (a.x + b.x) / 2 - rect.left - rect.width / 2 - (pinch.x - pinch.panX) * ratio
  panY = (a.y + b.y) / 2 - rect.top - rect.height / 2 - (pinch.y - pinch.panY) * ratio
  zoom.value = scale
  zoomed.value = scale > 1
  if (event.cancelable) event.preventDefault()
  if (!frame) frame = requestAnimationFrame(() => { frame = 0; paintZoom() })
  emit('zoomGesture')
  emit('zoomChange', zoomed.value)
}
function finishPinch(event: PointerEvent) {
  touchPointers.delete(event.pointerId)
  releasePointer(event.pointerId)
  suppressClickUntil = performance.now() + 500
  if (!touchPointers.size) {
    pinch = null
    dragging.value = pressed.value = false
    paintZoom()
  }
}
function pointerDown(event: PointerEvent) {
  if (event.pointerType === 'touch' && !(event.target as Element).closest('video')) {
    touchPointers.set(event.pointerId, { x: event.clientX, y: event.clientY })
    if (touchPointers.size >= 2 && (pinch || beginPinch())) return
  }
  if (!event.isPrimary) { if (start) pointerCancel(); return }
  if (closing || awaitingNavigation || event.button !== 0 || (event.target as Element).closest('video')) return
  // A new gesture may interrupt a snapback instead of being ignored until it ends.
  if (animation && track.value) {
    position = new DOMMatrixReadOnly(getComputedStyle(track.value).transform).m41
    cancelMotion()
    paint()
  }
  pressed.value = true
  overImage.value = isImagePoint(event)
  dragPitch = pitch()
  const originX = event.clientX - position
  start = { x: originX, y: event.clientY, time: performance.now(), pointerId: event.pointerId, pointerType: event.pointerType }
  horizontal = false
  panStartX = panX
  panStartY = panY
}
function pointerMove(event: PointerEvent) {
  if (touchPointers.has(event.pointerId)) touchPointers.set(event.pointerId, { x: event.clientX, y: event.clientY })
  if (pinch) { movePinch(event); return }
  if (!start) {
    // Hit testing can read layout; do it at most once per frame and never while dragging.
    if (!hoverFrame) hoverFrame = requestAnimationFrame(() => {
      hoverFrame = 0
      if (!start) overImage.value = isImagePoint(event)
    })
    return
  }
  if (event.pointerId !== start.pointerId) return
  if (event.pointerType === 'mouse' && event.buttons === 0) { pointerCancel(event); return }
  const dx = event.clientX - start.x
  const dy = event.clientY - start.y
  if (zoomed.value) {
    if (Math.max(Math.abs(dx), Math.abs(dy)) < 4 && !dragging.value) return
    viewport.value?.setPointerCapture(event.pointerId)
    dragging.value = true
    panX = panStartX + dx
    panY = panStartY + dy
    if (event.cancelable) event.preventDefault()
    if (!frame) frame = requestAnimationFrame(() => { frame = 0; paintZoom() })
    return
  }
  if (!horizontal && (Math.abs(dx) < 4 || Math.abs(dx) <= Math.abs(dy))) return
  if (props.photos.length < 2) return
  const capture = !horizontal
  horizontal = true
  if (capture) viewport.value?.setPointerCapture(event.pointerId)
  dragging.value = true
  moving.value = true
  if (event.cancelable) event.preventDefault()
  position = Math.max(-dragPitch, Math.min(dragPitch, dx))
  // Update only the compositor transform, without rerendering the gallery on every pointer event.
  if (!frame) frame = requestAnimationFrame(() => { frame = 0; paint() })
}
function releasePointer(pointerId: number) {
  if (viewport.value?.hasPointerCapture(pointerId)) viewport.value.releasePointerCapture(pointerId)
}
function pointerUp(event: PointerEvent) {
  if (pinch) { finishPinch(event); return }
  touchPointers.delete(event.pointerId)
  const origin = start
  if (!origin || event.pointerId !== origin.pointerId) return
  start = null
  pressed.value = false
  dragging.value = false
  releasePointer(origin.pointerId)
  const dx = event.clientX - origin.x
  const dy = event.clientY - origin.y
  const moved = Math.max(Math.abs(dx), Math.abs(dy)) > 4
  if (moved) { suppressClickUntil = performance.now() + 450; lastTouchTap = null }
  if (!moved && !horizontal && origin.pointerType === 'touch' && isImagePoint(event)) {
    const now = performance.now()
    suppressClickUntil = now + 500
    if (lastTouchTap && now - lastTouchTap.time < 300 && Math.hypot(event.clientX - lastTouchTap.x, event.clientY - lastTouchTap.y) < 24) {
      lastTouchTap = null
      toggleZoom()
    } else lastTouchTap = { time: now, x: event.clientX, y: event.clientY }
    return
  }
  if (zoomed.value) {
    if (moved) suppressClickUntil = performance.now() + 450
    return
  }
  if (!horizontal) {
    if (origin.pointerType !== 'mouse' && Math.abs(dy) > 90 && Math.abs(dy) > Math.abs(dx) * 1.25) emit('dismiss')
    return
  }
  suppressClickUntil = performance.now() + 450
  const velocity = Math.abs(dx) / Math.max(1, performance.now() - origin.time)
  if (Math.abs(dx) > Math.min(90, pitch() * .2) || (Math.abs(dx) > 16 && velocity > .4)) {
    awaitingNavigation = true
    emit('navigate', dx < 0 ? 1 : -1)
  } else {
    void moveTo(0)
  }
}
function pointerCancel(event?: PointerEvent) {
  if (pinch && event) { finishPinch(event); return }
  if (event) touchPointers.delete(event.pointerId)
  if (!event) { touchPointers.clear(); pinch = null; lastTouchTap = null; dragging.value = pressed.value = false }
  if (!start || (event && event.pointerId !== start.pointerId)) return
  const pointerId = start.pointerId
  start = null
  horizontal = false
  pressed.value = false
  dragging.value = false
  releasePointer(pointerId)
  suppressClickUntil = performance.now() + 450
  if (zoomed.value) { cancelAnimationFrame(frame); frame = 0; paintZoom() }
  else void moveTo(0)
}
function pointerCaptureLost(event: PointerEvent) {
  // Touch starts with implicit capture on the image. Transferring it to the
  // viewport fires a bubbling lostpointercapture on that image, not a cancel.
  if (event.target !== viewport.value || viewport.value?.hasPointerCapture(event.pointerId)) return
  pointerCancel(event)
}
function pointerLeave(event: PointerEvent) {
  cancelAnimationFrame(hoverFrame)
  hoverFrame = 0
  overImage.value = false
  if (event.pointerType === 'mouse' && !horizontal) pointerCancel(event)
}
function isImagePoint(event: MouseEvent) {
  const image = (event.target as Element).closest('img')
  if (!image || !image.naturalWidth || !image.naturalHeight) return false
  const rect = image.getBoundingClientRect()
  const scale = Math.min(rect.width / image.naturalWidth, rect.height / image.naturalHeight)
  const width = image.naturalWidth * scale
  const height = image.naturalHeight * scale
  const left = rect.left + (rect.width - width) / 2
  const top = rect.top + (rect.height - height) / 2
  return event.clientX >= left && event.clientX <= left + width && event.clientY >= top && event.clientY <= top + height
}
function backdropClick(event: MouseEvent) {
  if (performance.now() < suppressClickUntil) return
  if ((event.target as Element).closest('video')) return
  if (isImagePoint(event)) { toggleZoom(); return }
  if (!zoomed.value) emit('dismiss')
}
function wheel(event: WheelEvent) {
  if (!matchesWheelZoom(event, props.wheelZoomModifier)) return
  emit('zoomGesture')
  event.preventDefault()
  event.stopPropagation()
  zoomWithWheel(event)
}
function windowBlur() { pointerCancel() }
onMounted(() => {
  window.addEventListener('blur', windowBlur)
  viewport.value?.addEventListener('wheel', wheel, { passive: false, capture: true })
})
onBeforeUnmount(() => {
  disposed = true
  window.removeEventListener('blur', windowBlur)
  viewport.value?.removeEventListener('wheel', wheel, { capture: true })
  cancelMotion()
  cancelAnimationFrame(hoverFrame)
  closeAnimation?.cancel()
})
</script>

<template>
  <div ref="viewport" class="lightbox-stage" :class="{ 'is-dragging': dragging, 'is-moving': moving, 'is-zoomed': zoomed, 'is-pressed': pressed, 'is-over-image': overImage }" :aria-busy="loading" @click="backdropClick"
    @pointerdown="pointerDown" @pointermove="pointerMove" @pointerup="pointerUp"
    @pointerleave="pointerLeave"
    @pointercancel="pointerCancel" @lostpointercapture="pointerCaptureLost">
    <div v-if="loading" class="lightbox-loading" role="status" aria-label="图片加载中">
      <svg class="lightbox-loading-spinner" viewBox="0 0 36 36" fill="none" aria-hidden="true">
        <circle cx="18" cy="18" r="16" stroke="currentColor" stroke-opacity=".25" stroke-width="4" />
        <path d="M18 2 A16 16 0 0 1 34 18" stroke="currentColor" stroke-width="4" stroke-linecap="round" />
      </svg>
    </div>
    <div ref="track" class="lightbox-track">
      <div v-for="item in slides" :key="item.key" class="lightbox-slide"
        :style="{ left: `calc(${item.offset * 100}% + ${item.offset} * var(--lightbox-slide-gap))` }"
        :aria-hidden="item.offset !== 0" :inert="item.offset !== 0">
        <video v-if="item.photo.mediaType === 'video' && item.offset === 0" class="lightbox-image lightbox-video"
          :src="item.photo.original" controls autoplay @ended="emit('ended')"></video>
        <DecodedPreviewImage v-else-if="item.photo.mediaType === 'image'" :photo="item.photo" :decorative="item.offset !== 0"
          @ready="emit('loaded', $event)" @failed="emit('failed', $event)" />
        <img v-else class="lightbox-image" :src="item.photo.thumbnail" alt="" draggable="false" />
      </div>
    </div>
  </div>
</template>
