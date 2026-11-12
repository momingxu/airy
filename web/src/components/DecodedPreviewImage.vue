<script setup lang="ts">
import { ref, watch } from 'vue'
import type { Photo } from '../types/gallery'

const props = defineProps<{ photo: Photo; decorative?: boolean }>()
const emit = defineEmits<{ ready: [id: string]; failed: [id: string] }>()
const original = ref<HTMLImageElement | null>(null)
const ready = ref(false)
let generation = 0
watch(() => props.photo.original, () => {
  generation += 1
  ready.value = false
}, { immediate: true, flush: 'sync' })

async function reveal(event: Event) {
  const image = event.currentTarget as HTMLImageElement
  const version = generation
  try {
    // Keep progressive network/decoder updates hidden until the whole image is ready.
    await image.decode()
  } catch {
    if (version === generation && image === original.value) emit('failed', props.photo.id)
    return
  }
  if (version !== generation || image !== original.value) return
  ready.value = true
  emit('ready', props.photo.id)
}
function failed(event: Event) {
  if (event.currentTarget !== original.value) return
  ready.value = false
  emit('failed', props.photo.id)
}
</script>

<template>
  <img ref="original" :key="photo.original" class="lightbox-image lightbox-decoded-original" :class="{ ready }"
    :src="photo.original" :alt="decorative ? '' : photo.name" decoding="async" draggable="false"
    @load="reveal" @error="failed" />
</template>
