<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'

const props = defineProps<{ active: boolean }>()
const playing = ref(false)
const burst = ref(0)
let cleanup: ReturnType<typeof setTimeout> | undefined
const heart = 'M12 21s-8.5-5.2-8.5-11.4A4.8 4.8 0 0 1 12 6.5a4.8 4.8 0 0 1 8.5 3.1C20.5 15.8 12 21 12 21Z'
const colors = ['var(--accent)', 'color-mix(in srgb, var(--accent) 70%, white)', 'var(--accent-strong)', 'color-mix(in srgb, var(--accent) 45%, white)']
// Seven fine particle pairs keep the burst compact around the heart.
const particles = Array.from({ length: 14 }, (_, index) => {
  const angle = (Math.floor(index / 2) / 7 + (index % 2) * .025) * Math.PI * 2
  const distance = index % 2 ? 19 : 24
  const x = Math.cos(angle) * distance
  const y = Math.sin(angle) * distance
  return {
    radius: index % 2 ? .65 : 1.05,
    color: colors[index % colors.length],
    style: { '--x1': `${x * .72}px`, '--y1': `${y * .72}px`, '--x2': `${x}px`, '--y2': `${y}px` },
  }
})

watch(() => props.active, (active) => {
  clearTimeout(cleanup)
  playing.value = false
  if (!active || window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
  // A new key restarts the animation even during an unfinished burst.
  burst.value++
  playing.value = true
  cleanup = setTimeout(() => { playing.value = false }, 700)
})
onBeforeUnmount(() => clearTimeout(cleanup))
</script>

<template>
  <svg :class="['md-icon', 'favorite-icon', { active, liked: playing }]" viewBox="0 0 24 24" aria-hidden="true">
    <path :key="`heart-${burst}`" class="favorite-heart" :d="heart" />
    <g v-if="playing" :key="burst" class="favorite-effects">
      <circle class="favorite-ring" cx="12" cy="12" r="10" />
      <circle v-for="(particle, index) in particles" :key="index" class="favorite-particle"
        cx="12" cy="12" :r="particle.radius" :fill="particle.color" :style="particle.style" />
    </g>
  </svg>
</template>

<style scoped>
.favorite-icon { display: block; overflow: visible; fill: none; stroke: none; pointer-events: none; }
.favorite-heart { fill: transparent; stroke: currentColor; stroke-width: 1.8; stroke-linejoin: round; transform-origin: 12px 12px; transition: fill 100ms, color 100ms; }
.active .favorite-heart { fill: currentColor; stroke-width: 0; }
.liked .favorite-heart { animation: favorite-pop 520ms cubic-bezier(.22,.68,.3,1) both; }
.favorite-ring { fill: none; stroke: var(--accent); stroke-width: 1.2; transform-origin: 12px 12px; animation: favorite-ring 380ms cubic-bezier(.16,1,.3,1) both; }
.favorite-particle { stroke: none; transform-origin: 12px 12px; animation: favorite-particle 620ms cubic-bezier(.16,.7,.3,1) both; }
@keyframes favorite-pop {
  0% { transform: scale(.72); }
  38% { transform: scale(1.16); }
  68% { transform: scale(.96); }
  100% { transform: scale(1); }
}
@keyframes favorite-ring {
  0% { transform: scale(.65); opacity: .5; }
  100% { transform: scale(1.45); opacity: 0; }
}
@keyframes favorite-particle {
  0% { transform: translate(0,0) scale(0); opacity: 0; }
  18% { opacity: .85; }
  35% { transform: translate(var(--x1),var(--y1)) scale(1); opacity: .85; }
  100% { transform: translate(var(--x2),var(--y2)) scale(.2); opacity: 0; }
}
@media (prefers-reduced-motion: reduce) {
  .favorite-heart { animation: none !important; transition: none; }
  .favorite-effects { display: none; }
}
</style>
