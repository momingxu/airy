<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { Photo } from '../types/gallery'

const props = defineProps<{ photos: Photo[]; index: number; loading: boolean }>()
const emit = defineEmits<{
  navigate: [direction: number]
  dismiss: []
  loaded: [event: Event]
  failed: [event: Event]
  ended: []
}>()
const viewport = ref<HTMLDivElement | null>(null)
const track = ref<HTMLDivElement | null>(null)
const dragging = ref(false)
const moving = ref(false)
const slides = computed(() => {
  const offsets = props.photos.length < 2 ? [0] : [-1, 0, 1]
  return offsets.map(offset => {
    const index = (props.index + offset + props.photos.length) % props.photos.length
    return { offset, photo: props.photos[index], key: `${index}${props.photos.length === 2 && offset === -1 ? '-copy' : ''}` }
  })
})
let position = 0
let frame = 0
let animation: Animation | null = null
let start: { x: number; y: number; time: number; pointerId: number; pointerType: string } | null = null
let horizontal = false
let awaitingNavigation = false
let suppressClickUntil = 0
let disposed = false

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
  track.value?.querySelectorAll('video').forEach(video => video.pause())
  await moveTo(-direction * pitch())
}
defineExpose({ slide })
watch(() => props.index, () => {
  cancelMotion()
  position = 0
  moving.value = false
  awaitingNavigation = false
  paint()
}, { flush: 'post' })

function pointerDown(event: PointerEvent) {
  if (!event.isPrimary) { if (start) pointerCancel(); return }
  if (props.loading || awaitingNavigation || animation || event.button !== 0 || (event.target as Element).closest('video')) return
  start = { x: event.clientX, y: event.clientY, time: performance.now(), pointerId: event.pointerId, pointerType: event.pointerType }
  horizontal = false
}
function pointerMove(event: PointerEvent) {
  if (!start || event.pointerId !== start.pointerId) return
  const dx = event.clientX - start.x
  const dy = event.clientY - start.y
  if (!horizontal && (Math.abs(dx) < 4 || Math.abs(dx) <= Math.abs(dy))) return
  if (props.photos.length < 2) return
  const capture = !horizontal
  horizontal = true
  if (capture) viewport.value?.setPointerCapture(event.pointerId)
  dragging.value = true
  moving.value = true
  if (event.cancelable) event.preventDefault()
  position = Math.max(-pitch(), Math.min(pitch(), dx))
  // Update only the compositor transform, without rerendering the gallery on every pointer event.
  if (!frame) frame = requestAnimationFrame(() => { frame = 0; paint() })
}
function releasePointer(pointerId: number) {
  if (viewport.value?.hasPointerCapture(pointerId)) viewport.value.releasePointerCapture(pointerId)
}
function pointerUp(event: PointerEvent) {
  const origin = start
  if (!origin || event.pointerId !== origin.pointerId) return
  start = null
  dragging.value = false
  releasePointer(origin.pointerId)
  const dx = event.clientX - origin.x
  const dy = event.clientY - origin.y
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
  if (!start || (event && event.pointerId !== start.pointerId)) return
  const pointerId = start.pointerId
  start = null
  horizontal = false
  dragging.value = false
  releasePointer(pointerId)
  suppressClickUntil = performance.now() + 450
  void moveTo(0)
}
function pointerCaptureLost(event: PointerEvent) {
  // Touch starts with implicit capture on the image. Transferring it to the
  // viewport fires a bubbling lostpointercapture on that image, not a cancel.
  if (event.target !== viewport.value || viewport.value?.hasPointerCapture(event.pointerId)) return
  pointerCancel(event)
}
function pointerLeave(event: PointerEvent) {
  if (event.pointerType === 'mouse' && !horizontal) pointerCancel(event)
}
function backdropClick(event: MouseEvent) {
  if (performance.now() < suppressClickUntil) return
  if (!(event.target as Element).closest('img, video')) emit('dismiss')
}
onBeforeUnmount(() => { disposed = true; cancelMotion() })
</script>

<template>
  <div ref="viewport" class="lightbox-stage" :class="{ 'is-dragging': dragging, 'is-moving': moving }" :aria-busy="loading" @click="backdropClick"
    @pointerdown="pointerDown" @pointermove="pointerMove" @pointerup="pointerUp"
    @pointerleave="pointerLeave"
    @pointercancel="pointerCancel" @lostpointercapture="pointerCaptureLost">
    <div v-if="loading" class="lightbox-loading" role="status" aria-label="图片加载中">
      <span class="lightbox-loading-spinner" aria-hidden="true"></span>
    </div>
    <div ref="track" class="lightbox-track">
      <div v-for="item in slides" :key="item.key" class="lightbox-slide"
        :style="{ left: `calc(${item.offset * 100}% + ${item.offset} * var(--lightbox-slide-gap))` }"
        :aria-hidden="item.offset !== 0" :inert="item.offset !== 0">
        <video v-if="item.photo.mediaType === 'video' && item.offset === 0" class="lightbox-image lightbox-video"
          :src="item.photo.original" controls autoplay @ended="emit('ended')"></video>
        <img v-else class="lightbox-image" :src="item.photo.mediaType === 'image' ? item.photo.original : item.photo.thumbnail"
          :alt="item.offset === 0 ? item.photo.name : ''" :data-photo-id="item.photo.id" decoding="async" draggable="false"
          @load="emit('loaded', $event)" @error="emit('failed', $event)" />
      </div>
    </div>
  </div>
</template>
