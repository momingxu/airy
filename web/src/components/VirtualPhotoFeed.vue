<script setup lang="ts">
import FavoriteIcon from "./FavoriteIcon.vue"
import { useVirtualizer } from '@tanstack/vue-virtual'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { Photo, PhotoGroup } from '../types/gallery'
import MdIcon from './MdIcon.vue'

const props = withDefaults(defineProps<{
  groups: PhotoGroup[]
  scrollElement: HTMLElement | null
  editablePersonId?: number | null
  editableSeriesId?: number | null
  personAlbumSavingKeys?: Set<string>
  seriesAlbumSavingKeys?: Set<string>
}>(), {
  editablePersonId: null,
  editableSeriesId: null,
  personAlbumSavingKeys: () => new Set<string>(),
  seriesAlbumSavingKeys: () => new Set<string>(),
})

const emit = defineEmits<{
  addPersonAlbum: [group: PhotoGroup]
  openPhoto: [photo: Photo, group: PhotoGroup]
  openPerson: [personID: number]
  openSeries: [seriesID: number]
  removePersonAlbum: [personID: number, group: PhotoGroup]
  removeSeriesAlbum: [seriesID: number, group: PhotoGroup]
  toggleAlbumFavorite: [group: PhotoGroup]
  togglePhotoFavorite: [photo: Photo]
}>()

interface HeaderItem {
  key: string
  kind: 'header'
  size: number
  group: PhotoGroup
}

interface PhotoRowItem {
  key: string
  kind: 'photos'
  size: number
  height: number
  photos: Photo[]
  widths: number[]
  group: PhotoGroup
}

interface SpacerItem {
  key: string
  kind: 'spacer'
  size: number
}

type FeedItem = HeaderItem | PhotoRowItem | SpacerItem

const feed = ref<HTMLElement | null>(null)
const feedWidth = ref(0)
const viewportHeight = ref(0)
const scrollMargin = ref(0)
const overscanRows = computed(() => {
  if (feedWidth.value <= 0 || feedWidth.value > 760) return 12
  const rowHeight = Math.max(1, (feedWidth.value - 6) / 3 + 3)
  // Keep roughly four screens of mobile thumbnails ready in each direction,
  // allowing extra rows for album headers and spacing.
  return Math.min(80, Math.max(24, Math.ceil(viewportHeight.value * 4 / rowHeight) + 4))
})
let resizeObserver: ResizeObserver | undefined

function buildPhotoRows(group: PhotoGroup, width: number, mobile: boolean): PhotoRowItem[] {
  const rows: PhotoRowItem[] = []
  if (!group.photos.length || width <= 0) return rows

  const gap = mobile ? 3 : 5
  if (mobile) {
    const pending = [...group.photos]
    const isPortrait = (photo: Photo) => photo.aspectRatio > 0 && photo.aspectRatio < 1
    for (let index = 0; pending.length;) {
      // Anchor each row to the earliest photo, with swaps limited to the next five.
      const nearby = pending.slice(0, 5)
      const portraitPositions = nearby.flatMap((photo, position) => isPortrait(photo) ? [position] : [])
      let selected: number[]
      if (isPortrait(pending[0]) && portraitPositions.length >= 3) {
        selected = portraitPositions.slice(0, 3)
      } else {
        // Prefer two landscapes (squares included), or one portrait + one landscape.
        const partner = nearby.findIndex((photo, position) => position > 0 && !isPortrait(photo))
        selected = pending.length === 1 ? [0] : [0, partner > 0 ? partner : 1]
      }
      const photos = selected.map((position) => pending[position])
      const columns = photos.length === 3 || (pending.length < 3 && photos.every(isPortrait)) ? 3 : 2
      for (const position of [...selected].reverse()) pending.splice(position, 1)
      const ratios = photos.map((photo) => Number.isFinite(photo.aspectRatio) && photo.aspectRatio > 0 ? photo.aspectRatio : 1)
      const ratioSum = ratios.reduce((sum, ratio) => sum + ratio, 0)
      // Reserve missing slots so an incomplete last row does not stretch across the screen.
      const height = Math.max(1, (width - gap * (columns - 1)) / (ratioSum * columns / photos.length))
      rows.push({
        key: `${group.id}-row-${index}`,
        kind: 'photos',
        size: height + gap,
        height,
        photos,
        widths: ratios.map((ratio) => ratio * height),
        group,
      })
      index += photos.length
    }
    return rows
  }

  const targetHeight = 290
  let rowPhotos: Photo[] = []
  let ratioSum = 0
  let rowStart = 0

  const appendRow = (justify: boolean) => {
    if (!rowPhotos.length) return
    const availableWidth = width - gap * (rowPhotos.length - 1)
    const height = justify ? availableWidth / ratioSum : targetHeight
    const widths = rowPhotos.map((photo) => photo.aspectRatio * height)
    if (justify && widths.length) {
      const usedWidth = widths.reduce((total, itemWidth) => total + itemWidth, 0)
      widths[widths.length - 1] += availableWidth - usedWidth
    }
    rows.push({
      key: `${group.id}-row-${rowStart}`,
      kind: 'photos',
      size: height + gap,
      height,
      photos: rowPhotos,
      widths,
      group,
    })
    rowPhotos = []
    ratioSum = 0
  }

  group.photos.forEach((photo, index) => {
    if (!rowPhotos.length) rowStart = index
    rowPhotos.push(photo)
    ratioSum += photo.aspectRatio
    const gapsWidth = gap * (rowPhotos.length - 1)
    if (ratioSum * targetHeight + gapsWidth >= width) appendRow(true)
  })
  appendRow(false)
  return rows
}

const layoutItems = computed<FeedItem[]>(() => {
  const width = feedWidth.value
  if (!width) return []
  const mobile = width <= 760
  const items: FeedItem[] = []
  for (const group of props.groups) {
    items.push({
      key: `${group.id}-header`,
      kind: 'header',
      size: mobile ? 68 : 82,
      group,
    })
    items.push(...buildPhotoRows(group, width, mobile))
    items.push({ key: `${group.id}-spacer`, kind: 'spacer', size: mobile ? 34 : 52 })
  }
  return items
})

const virtualizerOptions = computed(() => ({
  count: layoutItems.value.length,
  getScrollElement: () => props.scrollElement,
  estimateSize: (index: number) => layoutItems.value[index]?.size ?? 290,
  getItemKey: (index: number) => layoutItems.value[index]?.key ?? index,
  overscan: overscanRows.value,
  scrollMargin: scrollMargin.value,
}))

const rowVirtualizer = useVirtualizer(virtualizerOptions)
const virtualRows = computed(() => rowVirtualizer.value.getVirtualItems())
const totalSize = computed(() => rowVirtualizer.value.getTotalSize())
const renderedRows = computed(() =>
  virtualRows.value.flatMap((virtualRow) => {
    const item = layoutItems.value[virtualRow.index]
    return item ? [{ virtualRow, item }] : []
  }),
)

function updateMeasurements() {
  const element = feed.value
  const scroller = props.scrollElement
  if (!element) return
  feedWidth.value = element.clientWidth
  if (scroller) {
    viewportHeight.value = scroller.clientHeight
    scrollMargin.value =
      element.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop
  }
}

function itemTransform(start: number) {
  return `translateY(${start - scrollMargin.value}px)`
}

function measureFeedItem(element: unknown) {
  if (element instanceof Element) rowVirtualizer.value.measureElement(element)
}

function photoWidth(item: PhotoRowItem, index: number) {
  return { width: `${item.widths[index]}px`, height: `${item.height}px` }
}

function markImageLoaded(event: Event) {
  const image = event.currentTarget as HTMLImageElement
  image.classList.add('loaded')
}

watch(
  () => props.scrollElement,
  (element, previous) => {
    if (previous) resizeObserver?.unobserve(previous)
    if (element) resizeObserver?.observe(element)
    nextTick(updateMeasurements)
  },
)

// Metadata changes (such as favorites) replace group objects without changing
// geometry. Preserve measured header heights unless the row layout changes.
const layoutGeometry = computed(() => JSON.stringify([
  feedWidth.value,
  layoutItems.value.map((item) => [item.key, item.size]),
]))

watch(layoutGeometry, () => {
  nextTick(() => rowVirtualizer.value.measure())
})

onMounted(() => {
  updateMeasurements()
  if (feed.value) {
    resizeObserver = new ResizeObserver(updateMeasurements)
    resizeObserver.observe(feed.value)
    if (props.scrollElement) resizeObserver.observe(props.scrollElement)
  }
})

onBeforeUnmount(() => resizeObserver?.disconnect())
</script>

<template>
  <div ref="feed" class="virtual-photo-feed" :style="{ height: `${totalSize}px` }">
    <div
      v-for="{ virtualRow, item } in renderedRows"
      :key="String(virtualRow.key)"
      class="virtual-feed-item"
      :class="{ 'virtual-feed-header': item.kind === 'header' }"
      :data-index="virtualRow.index"
      :ref="measureFeedItem"
      :style="{
        height: item.kind === 'header' ? undefined : `${item.size}px`,
        minHeight: `${item.size}px`,
        transform: itemTransform(virtualRow.start),
      }"
    >
      <template v-if="item.kind === 'header'">
        <header class="virtual-section-header">
          <div class="virtual-section-copy">
            <h2>{{ item.group.name }}</h2>
            <div class="album-meta-row">
              <button
                v-if="item.group.canFavorite"
                :class="['album-favorite-button', { saved: item.group.favorite }]"
                type="button"
                :aria-label="item.group.favorite ? `取消收藏相册 ${item.group.name}` : `收藏相册 ${item.group.name}`"
                :aria-pressed="item.group.favorite"
                @click="emit('toggleAlbumFavorite', item.group)"
              >
                <FavoriteIcon :key="item.group.albumID" :active="item.group.favorite" />
              </button>
              <p class="album-date-count">
                <template v-if="item.group.publishedDate">
                  <MdIcon name="calendar" />
                  <time :datetime="item.group.publishedDate" aria-label="发布日期">{{ item.group.publishedDate.replace(/-/g, '.') }}</time>
                  <span aria-hidden="true">·</span>
                </template>
                <span>{{ item.group.count }}P</span>
              </p>
              <div v-if="item.group.people.length" class="album-related-people">
                <template v-for="person in item.group.people" :key="person.id">
                  <span
                    v-if="props.editablePersonId === person.id"
                    class="album-person-chip removable"
                  >
                    <button class="album-person-link" type="button" @click="emit('openPerson', person.id)">
                      {{ person.name }}<template v-if="person.note"> · {{ person.note }}</template>
                    </button>
                    <button
                      class="album-person-remove"
                      type="button"
                      :aria-label="`从 ${person.name} 中移除相册 ${item.group.name}`"
                      :disabled="props.personAlbumSavingKeys.has(`${person.id}:${item.group.albumID}`)"
                      @click="emit('removePersonAlbum', person.id, item.group)"
                    >
                      −
                    </button>
                  </span>
                  <button
                    v-else
                    class="album-person-chip album-person-link"
                    type="button"
                    @click="emit('openPerson', person.id)"
                  >
                    {{ person.name }}<template v-if="person.note"> · {{ person.note }}</template>
                  </button>
                </template>
              </div>
              <button
                v-else-if="item.group.canFavorite"
                class="album-person-add"
                type="button"
                :aria-label="`为相册 ${item.group.name} 关联模特`"
                title="关联模特"
                @click="emit('addPersonAlbum', item.group)"
              >
                +
              </button>
              <div v-if="item.group.series.length" class="album-related-people album-related-series">
                <template v-for="seriesItem in item.group.series" :key="seriesItem.id">
                  <span
                    v-if="props.editableSeriesId === seriesItem.id"
                    class="album-person-chip album-series-chip removable"
                  >
                    <button class="album-person-link" type="button" @click="emit('openSeries', seriesItem.id)">
                      {{ seriesItem.name }}
                    </button>
                    <button
                      class="album-person-remove"
                      type="button"
                      :aria-label="`从 ${seriesItem.name} 中移除相册 ${item.group.name}`"
                      :disabled="props.seriesAlbumSavingKeys.has(`${seriesItem.id}:${item.group.albumID}`)"
                      @click="emit('removeSeriesAlbum', seriesItem.id, item.group)"
                    >
                      −
                    </button>
                  </span>
                  <button
                    v-else
                    class="album-person-chip album-series-chip album-person-link"
                    type="button"
                    @click="emit('openSeries', seriesItem.id)"
                  >
                    {{ seriesItem.name }}
                  </button>
                </template>
              </div>
            </div>
          </div>
        </header>
      </template>

      <div
        v-else-if="item.kind === 'photos'"
        class="virtual-photo-row"
        :style="{ height: `${item.height}px` }"
      >
        <article
          v-for="(photo, photoIndex) in item.photos"
          :key="photo.id"
          class="photo-card"
          :style="photoWidth(item, photoIndex)"
        >
          <button
            class="photo-open"
            type="button"
            :aria-label="`预览 ${photo.name}`"
            @click="emit('openPhoto', photo, item.group)"
          >
            <img
              class="photo-image"
              :src="photo.thumbnail"
              :alt="photo.name"
              @load="markImageLoaded"
            />
            <span class="photo-caption">{{ photo.name.replace('.jpg', '') }}</span>
          </button>
          <button
            :class="['favorite-button', { saved: photo.favorite }]"
            type="button"
            :aria-label="photo.favorite ? `取消收藏图片 ${photo.name}` : `收藏图片 ${photo.name}`"
            :aria-pressed="photo.favorite"
            @click="emit('togglePhotoFavorite', photo)"
          >
            <FavoriteIcon :key="photo.id" :active="photo.favorite" />
          </button>
        </article>
      </div>
    </div>
  </div>
</template>
