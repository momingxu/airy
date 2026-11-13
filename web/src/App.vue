<script setup lang="ts">
import { isWheelZoomModifier, matchesWheelZoom, wheelZoomLabels, type WheelZoomModifier } from './utils/wheelZoom'
import LightboxSlides from './components/LightboxSlides.vue'
import FavoriteIcon from "./components/FavoriteIcon.vue"
import AlbumCsvActions from "./components/AlbumCsvActions.vue"
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import MdIcon from './components/MdIcon.vue'
import {
  fetchAuthStatus,
  login,
  logout,
  setupAccount,
  updateAccount,
} from './api/auth'
import {
  cancelScan,
  createDirectory,
  deleteDirectory,
  fetchDirectories,
  fetchScanSchedule,
  scanAllDirectories,
  scanDirectory,
  updateDirectorySettings,
  updateScanSchedule,
  type ResourceDirectory,
  type ScanSchedule,
  type ScanStatus,
} from './api/directories'
import {
  fetchFavoritePhotos,
  fetchGallery,
  updateAlbumFavorite,
  updatePhotoFavorite,
  type AlbumItem,
  type MediaItem,
} from './api/gallery'
import {
  addPersonAlbum,
  createPerson,
  deletePerson,
  fetchPeople,
  fetchPersonGallery,
  fetchPersonAlbumOptions,
  removePersonAlbum,
  searchAlbums,
  updatePerson,
  updatePersonFavorite,
  type AlbumSearchItem,
  type PersonItem,
  type PersonAlbumOption,
} from './api/people'
import {
  createSeries,
  deleteSeries,
  fetchSeries,
  fetchSeriesGallery,
  fetchSeriesAlbumOptions,
  removeSeriesAlbum,
  updateSeries,
  type SeriesItem,
} from './api/series'
import VirtualPhotoFeed from './components/VirtualPhotoFeed.vue'
import {
  applyMaterialTheme,
  DEFAULT_THEME_SEED,
  normalizeThemeSeed,
  THEME_PRESETS,
  THEME_SEED_STORAGE_KEY,
} from './theme'
import type { Photo, PhotoGroup } from './types/gallery'

type ViewName = 'photos' | 'people' | 'series' | 'favorites' | 'folders' | 'settings'
type ApiState = 'loading' | 'online' | 'offline'
type AuthState = 'loading' | 'setup' | 'login' | 'authenticated'
type ConfirmationOptions = {
  title: string
  message: string
  confirmLabel?: string
}

const groups = ref<PhotoGroup[]>([])

const activeView = ref<ViewName>('favorites')
const authState = ref<AuthState>('loading')
const authUsername = ref('')
const authPassword = ref('')
const authPasswordConfirm = ref('')
const authError = ref('')
const authSaving = ref(false)
const accountUsername = ref('')
const accountCurrentPassword = ref('')
const accountNewPassword = ref('')
const accountNewPasswordConfirm = ref('')
const accountError = ref('')
const accountMessage = ref('')
const accountSaving = ref(false)
const defaultSearchShortcut = 'Alt'
const searchShortcutStorageKey = 'airy-search-shortcut'
const searchShortcut = ref(defaultSearchShortcut)
const searchShortcutDraft = ref(defaultSearchShortcut)
const shortcutMessage = ref('')
const content = ref<HTMLElement | null>(null)
const apiState = ref<ApiState>('loading')
const lightbox = ref<HTMLDialogElement | null>(null)
const personCreateDialog = ref<HTMLDialogElement | null>(null)
const personEditDialog = ref<HTMLDialogElement | null>(null)
const albumPersonDialog = ref<HTMLDialogElement | null>(null)
const gallerySearchDialog = ref<HTMLDialogElement | null>(null)
const confirmationDialog = ref<HTMLDialogElement | null>(null)
const confirmationTitle = ref('')
const confirmationMessage = ref('')
const confirmationLabel = ref('确定')
const gallerySearchInputElement = ref<HTMLInputElement | null>(null)
const previewPhotos = ref<Photo[]>([])
const previewIndex = ref(0)
const wheelZoomModifier = ref<WheelZoomModifier>('ctrl')
const lightboxZoomed = ref(false)
const lightboxSlides = ref<InstanceType<typeof LightboxSlides> | null>(null)
const lightboxLoadingPhotoID = ref<string | null>(null)
let lightboxSwitchPending = false
let lightboxClosing = false
let queuedLightboxDirection = 0
let lightboxSwitchGeneration = 0
const slideshowPlaying = ref(false)
const slideshowProgressKey = ref(0)
const theme = ref<'light' | 'dark'>('light')
const themeSeed = ref(DEFAULT_THEME_SEED)
const themeSeedDraft = ref(DEFAULT_THEME_SEED)
const mobileMoreOpen = ref(false)
const totalAlbumCount = ref(0)
const totalMediaCount = ref(0)
const hasMoreAlbums = ref(false)
const galleryLoading = ref(true)
const galleryLoadingMore = ref(false)
const galleryError = ref('')
const gallerySearchInput = ref('')
const gallerySearch = ref('')
const favoriteGroups = ref<PhotoGroup[]>([])
const favoritePhotoGroup = ref<PhotoGroup | null>(null)
const favoritePhotoCount = ref(0)
const favoriteAlbumCount = ref(0)
const favoriteMediaCount = ref(0)
const favoriteHasMoreAlbums = ref(false)
const favoriteGalleryLoading = ref(false)
const favoriteGalleryLoadingMore = ref(false)
const favoriteGalleryError = ref('')
const favoriteSavingAlbumIDs = ref<Set<number>>(new Set())
const favoriteSavingPhotoIDs = ref<Set<string>>(new Set())
const directories = ref<ResourceDirectory[]>([])
const directoriesLoading = ref(false)
const directorySaving = ref(false)
const directoryPath = ref('')
const directoryAlbumDepth = ref(1)
const directoryError = ref('')
const activeScan = ref<ScanStatus | null>(null)
const scanSchedule = ref<ScanSchedule>({ enabled: false, scan_time: '01:00', timezone: 'Asia/Shanghai' })
const globalScanStarting = ref(false)
const scanControlChanging = ref(false)
const scanScheduleSaving = ref(false)
const directorySettingsSaving = ref<Set<number>>(new Set())
const albumDepthOptions = Array.from({ length: 10 }, (_, index) => index + 1)
const people = ref<PersonItem[]>([])
const selectedPersonID = ref<number | null>(null)
const personGroups = ref<PhotoGroup[]>([])
const personTotalMedia = ref(0)
const personHasMoreAlbums = ref(false)
const peopleLoading = ref(false)
const personGalleryLoading = ref(false)
const personGalleryLoadingMore = ref(false)
const personSaving = ref(false)
const favoriteSavingPersonIDs = ref<Set<number>>(new Set())
const personAlbumSavingKeys = ref<Set<string>>(new Set())
const peopleSearch = ref('')
const peopleFavoritesOnly = ref(false)
const albumPersonSearch = ref('')
const albumPersonTarget = ref<PhotoGroup | null>(null)
const albumPersonLoading = ref(false)
const albumPersonError = ref('')
const personName = ref('')
const personNote = ref('')
const personAliases = ref('')
const personEditName = ref('')
const personEditNote = ref('')
const personEditAliases = ref('')
const editPersonAlbumSearch = ref('')
const editPersonAlbumOptions = ref<PersonAlbumOption[]>([])
const editPersonAlbumLoading = ref(false)
const editPersonSelectedAlbumIDs = ref<Set<number>>(new Set())
const editPersonRemovedAlbumIDs = ref<Set<number>>(new Set())
const editPersonLastSearch = ref<string | null>(null)
const selectableEditPersonAlbums = computed(() => editPersonAlbumOptions.value)
const selectedEditPersonAlbumCount = computed(() => selectableEditPersonAlbums.value.filter(editPersonAlbumSelected).length)
const allEditPersonAlbumsSelected = computed(() => selectableEditPersonAlbums.value.length > 0 && selectedEditPersonAlbumCount.value === selectableEditPersonAlbums.value.length)
const someEditPersonAlbumsSelected = computed(() => selectedEditPersonAlbumCount.value > 0 && !allEditPersonAlbumsSelected.value)
const editPersonAlbumError = ref('')
const editingPersonID = ref<number | null>(null)
let editPersonAlbumSearchVersion = 0
const personError = ref('')
const newPersonAlbumOptions = ref<AlbumSearchItem[]>([])
const newPersonSelectedAlbumIDs = ref<Set<number>>(new Set())
const newPersonAlbumSearchLoading = ref(false)
const seriesCreateDialog = ref<HTMLDialogElement | null>(null)
const seriesEditDialog = ref<HTMLDialogElement | null>(null)
const series = ref<SeriesItem[]>([])
const selectedSeriesID = ref<number | null>(null)
const seriesGroups = ref<PhotoGroup[]>([])
const seriesTotalMedia = ref(0)
const seriesHasMoreAlbums = ref(false)
const seriesLoading = ref(false)
const seriesGalleryLoading = ref(false)
const seriesGalleryLoadingMore = ref(false)
const seriesSaving = ref(false)
const favoriteSavingSeriesIDs = ref<Set<number>>(new Set())
const seriesAlbumSavingKeys = ref<Set<string>>(new Set())
const seriesSearch = ref('')
const seriesFavoritesOnly = ref(false)
const seriesName = ref('')
const seriesEditName = ref('')
const editSeriesAlbumSearch = ref('')
const editSeriesAlbumRegex = ref(false)
const editSeriesAlbumOptions = ref<PersonAlbumOption[]>([])
const editSeriesAlbumLoading = ref(false)
const editSeriesSelectedAlbumIDs = ref<Set<number>>(new Set())
const editSeriesRemovedAlbumIDs = ref<Set<number>>(new Set())
const editSeriesLastSearch = ref<string | null>(null)
const selectableEditSeriesAlbums = computed(() => editSeriesAlbumOptions.value)
const selectedEditSeriesAlbumCount = computed(() => selectableEditSeriesAlbums.value.filter(editSeriesAlbumSelected).length)
const allEditSeriesAlbumsSelected = computed(() => selectableEditSeriesAlbums.value.length > 0 && selectedEditSeriesAlbumCount.value === selectableEditSeriesAlbums.value.length)
const someEditSeriesAlbumsSelected = computed(() => selectedEditSeriesAlbumCount.value > 0 && !allEditSeriesAlbumsSelected.value)
const editSeriesAlbumError = ref('')
const editingSeriesID = ref<number | null>(null)
let editSeriesAlbumSearchVersion = 0
const seriesError = ref('')
const newSeriesAlbumOptions = ref<AlbumSearchItem[]>([])
const newSeriesSelectedAlbumIDs = ref<Set<number>>(new Set())
const newSeriesAlbumSearchLoading = ref(false)
let directoryRefreshTimer: number | undefined
let slideshowTimer: number | undefined
let newPersonSearchTimer: number | undefined
let newSeriesSearchTimer: number | undefined
let wheelLastSwitchAt = Number.NEGATIVE_INFINITY
let modifierShortcutChorded = false
let confirmationResolver: ((confirmed: boolean) => void) | null = null
let galleryLoadVersion = 0
const preloadedLightboxImages = new Map<string, HTMLImageElement>()

const slideshowDelay = 5000
const lightboxPreloadCount = 5

const viewMeta: Record<ViewName, { eyebrow: string; title: string }> = {
  photos: { eyebrow: 'LATEST', title: '最新' },
  people: { eyebrow: 'MODELS', title: '模特' },
  series: { eyebrow: 'SERIES', title: '系列' },
  favorites: { eyebrow: 'CURATED', title: '我的收藏' },
  folders: { eyebrow: 'LIBRARY SOURCES', title: '资源库' },
  settings: { eyebrow: 'SETTINGS', title: '设置' },
}

const viewPaths: Record<ViewName, string> = {
  photos: '/latest',
  people: '/people',
  series: '/series',
  favorites: '/',
  folders: '/folders',
  settings: '/settings',
}

const selectedPerson = computed(() =>
  people.value.find((person) => person.id === selectedPersonID.value),
)
const selectedSeries = computed(() =>
  series.value.find((item) => item.id === selectedSeriesID.value),
)
const searchShortcutLabel = computed(() => formatShortcutLabel(searchShortcut.value))
const searchShortcutDraftLabel = computed(() => formatShortcutLabel(searchShortcutDraft.value))
const duplicatePersonNames = computed(() => {
  const counts = new Map<string, number>()
  for (const person of people.value) {
    const key = person.name.trim().toLocaleLowerCase()
    counts.set(key, (counts.get(key) ?? 0) + 1)
  }
  return new Set([...counts].filter(([, count]) => count > 1).map(([name]) => name))
})
function personSearchText(person: PersonItem) {
  return [person.name, person.note, ...person.aliases].join(' ').toLocaleLowerCase()
}
function personIdentityLabel(person: PersonItem) {
  if (person.note) return person.note
  return duplicatePersonNames.value.has(person.name.trim().toLocaleLowerCase()) ? `模特 #${person.id}` : ''
}
function parsePersonAliases(value: string) {
  const aliases = value.split(/[\n,，]+/).map((item) => item.trim()).filter(Boolean)
  return [...new Map(aliases.map((alias) => [alias.toLocaleLowerCase(), alias])).values()]
}
const visibleSeries = computed(() => {
  const keywords = seriesSearch.value.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean)
  return series.value.filter((item) => {
    if (seriesFavoritesOnly.value && !item.favorite) return false
    const name = item.name.toLocaleLowerCase()
    return keywords.every((keyword) => name.includes(keyword))
  })
})
const visiblePeople = computed(() => {
  const keywords = peopleSearch.value.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean)
  return people.value.filter((person) => {
    if (peopleFavoritesOnly.value && !person.favorite) return false
    const content = personSearchText(person)
    return keywords.every((keyword) => content.includes(keyword))
  })
})
const albumPersonChoices = computed(() => {
  const associatedIDs = new Set(albumPersonTarget.value?.people.map((person) => person.id) ?? [])
  const keywords = albumPersonSearch.value.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean)
  return people.value.filter((person) => {
    if (associatedIDs.has(person.id)) return false
    const content = personSearchText(person)
    return keywords.every((keyword) => content.includes(keyword))
  })
})
const currentMeta = computed(() => {
  if (activeView.value === 'photos' && gallerySearch.value) {
    return { eyebrow: 'SEARCH RESULTS', title: `${gallerySearch.value}的结果` }
  }
  if (activeView.value === 'people' && selectedPerson.value) {
    return { eyebrow: 'MODELS / ALBUMS', title: selectedPerson.value.name }
  }
  if (activeView.value === 'series' && selectedSeries.value) {
    return { eyebrow: 'SERIES / ALBUMS', title: selectedSeries.value.name }
  }
  return viewMeta[activeView.value]
})
const visibleGroups = computed(() => {
  if (activeView.value === 'photos') return groups.value
  if (activeView.value === 'people') return personGroups.value
  if (activeView.value === 'series') return seriesGroups.value
  if (activeView.value === 'favorites') {
    return favoritePhotoGroup.value
      ? [favoritePhotoGroup.value, ...favoriteGroups.value]
      : favoriteGroups.value
  }
  return []
})
// Keep search/filter controls available when only the filtered results are empty.
const showOnlyEmptyState = computed(() => {
  if (activeView.value === 'people') {
    return !selectedPersonID.value && !peopleLoading.value && !personError.value && people.value.length === 0
  }
  if (activeView.value === 'series') {
    return !selectedSeriesID.value && !seriesLoading.value && !seriesError.value && series.value.length === 0
  }
  if (activeView.value === 'photos') {
    return !galleryLoading.value && !galleryError.value && groups.value.length === 0
  }
  if (activeView.value === 'favorites') {
    return !favoriteGalleryLoading.value && !favoriteGalleryError.value && visibleGroups.value.length === 0
  }
  return false
})
const summary = computed(() => {
  if (activeView.value === 'photos') {
    const totals = `${totalAlbumCount.value} 个相簿 · ${totalMediaCount.value} 个项目`
    return gallerySearch.value ? `“${gallerySearch.value}” · ${totals}` : totals
  }
  if (activeView.value === 'people') {
    if (!selectedPerson.value) return `${people.value.length} 个模特`
    const identity = personIdentityLabel(selectedPerson.value)
    const aliases = selectedPerson.value.aliases.length ? `别名：${selectedPerson.value.aliases.join('、')}` : ''
    return [identity, aliases, `${selectedPerson.value.album_count} 个相簿`, `${personTotalMedia.value} 个项目`].filter(Boolean).join(' · ')
  }
  if (activeView.value === 'series') {
    if (!selectedSeries.value) return `${series.value.length} 个系列`
    return `${selectedSeries.value.name} · ${selectedSeries.value.album_count} 个相簿 · ${seriesTotalMedia.value} 个项目`
  }
  if (activeView.value === 'favorites') {
    return `${favoritePhotoCount.value} 张收藏图片 · ${favoriteAlbumCount.value} 个收藏相簿 · ${favoriteMediaCount.value} 个相册项目`
  }
  if (activeView.value === 'folders') return `${directories.value.length} 个资源文件夹`
  if (activeView.value === 'settings') return `当前用户 · ${authUsername.value}`
  return ''
})
const directoryScanRunning = computed(() => activeScan.value?.state === 'scanning')
const currentPhoto = computed(() => previewPhotos.value[previewIndex.value])
const selectedNewPersonAlbumCount = computed(() =>
  newPersonAlbumOptions.value.reduce(
    (count, album) => count + (newPersonSelectedAlbumIDs.value.has(album.id) ? 1 : 0),
    0,
  ),
)
const allNewPersonAlbumsSelected = computed(() =>
  newPersonAlbumOptions.value.length > 0
  && selectedNewPersonAlbumCount.value === newPersonAlbumOptions.value.length,
)
const someNewPersonAlbumsSelected = computed(() =>
  selectedNewPersonAlbumCount.value > 0 && !allNewPersonAlbumsSelected.value,
)
const selectedNewSeriesAlbumCount = computed(() =>
  newSeriesAlbumOptions.value.reduce(
    (count, album) => count + (newSeriesSelectedAlbumIDs.value.has(album.id) ? 1 : 0),
    0,
  ),
)
const allNewSeriesAlbumsSelected = computed(() =>
  newSeriesAlbumOptions.value.length > 0
  && selectedNewSeriesAlbumCount.value === newSeriesAlbumOptions.value.length,
)
const someNewSeriesAlbumsSelected = computed(() =>
  selectedNewSeriesAlbumCount.value > 0 && !allNewSeriesAlbumsSelected.value,
)

function viewFromPath(pathname: string): ViewName {
  const normalizedPath = pathname.length > 1 ? pathname.replace(/\/+$/, '') : pathname
  if (normalizedPath === '/latest') return 'photos'
  if (normalizedPath === '/people' || /^\/people\/\d+$/.test(normalizedPath)) return 'people'
  if (normalizedPath === '/series' || /^\/series\/\d+$/.test(normalizedPath)) return 'series'
  if (normalizedPath === '/' || normalizedPath === '/favorites') return 'favorites'
  if (normalizedPath === '/folders') return 'folders'
  if (normalizedPath === '/settings') return 'settings'
  return 'favorites'
}

function setView(view: ViewName, updateHistory = true) {
  if (view === 'people') {
    void showPeopleList(updateHistory)
    return
  }
  if (view === 'series') {
    void showSeriesList(updateHistory)
    return
  }
  const previousView = activeView.value
  resetPersonDetail()
  resetSeriesDetail()
  activeView.value = view
  const path = view === 'photos' && gallerySearch.value
    ? `/latest?q=${encodeURIComponent(gallerySearch.value)}`
    : viewPaths[view]
  const currentPath = window.location.pathname + window.location.search
  if (updateHistory && currentPath !== path) window.history.pushState({}, '', path)
  if (content.value) content.value.scrollTop = 0
  if (view === 'photos' && previousView !== 'photos') void loadGallery(true)
  if (view === 'folders') void loadDirectories()
  if (view === 'favorites') void loadFavoriteGallery(true)
}

function showPhotosHome() {
  if (gallerySearch.value) {
    clearGallerySearch()
    return
  }
  if (activeView.value === 'photos') {
    if (content.value) content.value.scrollTop = 0
    return
  }
  setView('photos')
}

function applyGallerySearch() {
  const search = gallerySearchInput.value.trim().replace(/\s+/g, ' ')
  gallerySearchInput.value = search
  gallerySearch.value = search
  resetPersonDetail()
  resetSeriesDetail()
  activeView.value = 'photos'
  const path = search ? `/latest?q=${encodeURIComponent(search)}` : '/latest'
  if (window.location.pathname + window.location.search !== path) {
    window.history.pushState({}, '', path)
  }
  gallerySearchDialog.value?.close()
  if (content.value) content.value.scrollTop = 0
  void loadGallery(true)
}

function clearGallerySearch() {
  gallerySearchInput.value = ''
  if (!gallerySearch.value) return
  gallerySearch.value = ''
  resetPersonDetail()
  resetSeriesDetail()
  activeView.value = 'photos'
  if (window.location.pathname + window.location.search !== '/latest') {
    window.history.pushState({}, '', '/latest')
  }
  gallerySearchDialog.value?.close()
  if (content.value) content.value.scrollTop = 0
  void loadGallery(true)
}

async function openGallerySearchDialog() {
  gallerySearchInput.value = gallerySearch.value
  gallerySearchDialog.value?.showModal()
  await nextTick()
  gallerySearchInputElement.value?.focus()
  gallerySearchInputElement.value?.select()
}

function closeGallerySearchDialog() {
  gallerySearchDialog.value?.close()
}

function resetGallerySearchInput() {
  gallerySearchInput.value = gallerySearch.value
}

function closeGallerySearchFromBackdrop(event: MouseEvent) {
  if (event.target === event.currentTarget) closeGallerySearchDialog()
}

function formatScanTime(value?: string) {
  if (!value) return '尚未扫描'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

function formatScheduledTime(value?: string) {
  if (!value) return ''
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: scanSchedule.value.timezone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value))
}

function scanStageLabel(stage?: ScanStatus['stage']) {
  if (activeScan.value?.canceling) return '正在取消'
  switch (stage) {
    case 'discovering_albums': return '发现相册'
    case 'scanning_media': return '读取媒体信息'
    case 'generating_thumbnails': return '生成缩略图'
    default: return '扫描中'
  }
}

function scanStageDetail(scan: ScanStatus | null) {
  if (!scan) return ''
  if (scan.canceling) return '正在停止当前任务并清空扫描队列'
  if (scan.stage === 'discovering_albums') return `已发现 ${scan.stats.albums} 个相册`
  if (scan.stage === 'scanning_media') return `已读取 ${scan.stats.files} 个媒体文件`
  if (scan.stage === 'generating_thumbnails') return `${scan.stats.thumbnails_ready} / ${scan.stats.files} 个缩略图就绪`
  return ''
}

function scheduleDirectoryRefresh(delay = 5000) {
  window.clearTimeout(directoryRefreshTimer)
  if (document.visibilityState === 'hidden') return
  directoryRefreshTimer = window.setTimeout(() => void loadDirectories(true), delay)
}

async function loadDirectories(silent = false) {
  if (!silent) directoriesLoading.value = true
  directoryError.value = ''
  try {
    const response = await fetchDirectories()
    activeScan.value = response.scan
    directories.value = response.directories
    if (activeView.value === 'folders' && silent) void loadScanSchedule()
    if (response.scan.state === 'scanning') {
      scheduleDirectoryRefresh()
    } else {
      scheduleDirectoryRefresh(15000)
    }
  } catch (error) {
    directoryError.value = error instanceof Error ? error.message : '资源文件夹加载失败'
    scheduleDirectoryRefresh(15000)
  } finally {
    directoriesLoading.value = false
  }
}

async function addDirectory() {
  const path = directoryPath.value.trim()
  if (!path || directorySaving.value) return
  directorySaving.value = true
  directoryError.value = ''
  try {
    await createDirectory(path, directoryAlbumDepth.value)
    directoryPath.value = ''
    directoryAlbumDepth.value = 1
    await loadDirectories(true)
  } catch (error) {
    directoryError.value = error instanceof Error ? error.message : '添加资源文件夹失败'
  } finally {
    directorySaving.value = false
  }
}

async function startDirectoryScan(directory: ResourceDirectory) {
  directoryError.value = ''
  try {
    await scanDirectory(directory.id)
    directory.scanning = true
    await loadDirectories(true)
  } catch (error) {
    directoryError.value = error instanceof Error ? error.message : '启动扫描失败'
  }
}

async function startGlobalScan() {
  if (globalScanStarting.value || !directories.value.length) return
  globalScanStarting.value = true
  directoryError.value = ''
  try {
    await scanAllDirectories()
    await loadDirectories(true)
  } catch (error) {
    directoryError.value = error instanceof Error ? error.message : '启动全局扫描失败'
  } finally {
    globalScanStarting.value = false
  }
}

async function cancelActiveScan() {
  if (!directoryScanRunning.value || scanControlChanging.value) return
  scanControlChanging.value = true
  directoryError.value = ''
  try {
    const response = await cancelScan()
    activeScan.value = response.scan
    scheduleDirectoryRefresh(250)
  } catch (error) {
    directoryError.value = error instanceof Error ? error.message : '取消扫描失败'
  } finally {
    scanControlChanging.value = false
  }
}

async function loadScanSchedule() {
  try {
    const response = await fetchScanSchedule()
    scanSchedule.value = response.schedule
  } catch (error) {
    directoryError.value = error instanceof Error ? error.message : '读取定时扫描设置失败'
  }
}

async function saveScanSchedule(settings: Pick<ScanSchedule, 'enabled'>) {
  if (scanScheduleSaving.value) return
  scanScheduleSaving.value = true
  directoryError.value = ''
  try {
    const response = await updateScanSchedule(settings)
    scanSchedule.value = response.schedule
  } catch (error) {
    directoryError.value = error instanceof Error ? error.message : '保存定时扫描设置失败'
    await loadScanSchedule()
  } finally {
    scanScheduleSaving.value = false
  }
}

function changeScanScheduleEnabled(event: Event) {
  const input = event.currentTarget as HTMLInputElement
  void saveScanSchedule({ enabled: input.checked })
}

async function saveDirectorySettings(
  directory: ResourceDirectory,
  settings: Pick<ResourceDirectory, 'automation_disabled'>,
) {
  if (directorySettingsSaving.value.has(directory.id)) return
  directorySettingsSaving.value = new Set([...directorySettingsSaving.value, directory.id])
  directoryError.value = ''
  try {
    const response = await updateDirectorySettings(directory.id, settings)
    Object.assign(directory, response.directory)
    scheduleDirectoryRefresh()
  } catch (error) {
    directoryError.value = error instanceof Error ? error.message : '保存自动扫描设置失败'
    await loadDirectories(true)
  } finally {
    const next = new Set(directorySettingsSaving.value)
    next.delete(directory.id)
    directorySettingsSaving.value = next
  }
}

function changeDirectoryAutomation(directory: ResourceDirectory, event: Event) {
  const input = event.currentTarget as HTMLInputElement
  void saveDirectorySettings(directory, { automation_disabled: input.checked })
}

function requestConfirmation(options: ConfirmationOptions) {
  if (confirmationResolver || confirmationDialog.value?.open) return Promise.resolve(false)
  confirmationTitle.value = options.title
  confirmationMessage.value = options.message
  confirmationLabel.value = options.confirmLabel ?? '确定'
  return new Promise<boolean>((resolve) => {
    confirmationResolver = resolve
    void nextTick(() => {
      const dialog = confirmationDialog.value
      if (dialog) {
        dialog.showModal()
        return
      }
      confirmationResolver = null
      resolve(false)
    })
  })
}

function finishConfirmation(confirmed: boolean) {
  const resolve = confirmationResolver
  confirmationResolver = null
  if (confirmationDialog.value?.open) confirmationDialog.value.close()
  resolve?.(confirmed)
}

function handleConfirmationClosed() {
  if (!confirmationResolver) return
  const resolve = confirmationResolver
  confirmationResolver = null
  resolve(false)
}

function closeConfirmationFromBackdrop(event: MouseEvent) {
  if (event.target === event.currentTarget) finishConfirmation(false)
}

async function removeDirectory(directory: ResourceDirectory) {
  const confirmed = await requestConfirmation({
    title: '移除资源文件夹',
    message: `确定移除“${directory.name}”吗？\n只会清理 Airy 中的图库记录，不会删除磁盘文件。`,
    confirmLabel: '移除',
  })
  if (!confirmed) return
  directoryError.value = ''
  try {
    await deleteDirectory(directory.id)
    await Promise.all([loadDirectories(true), loadGallery(true)])
  } catch (error) {
    directoryError.value = error instanceof Error ? error.message : '移除资源文件夹失败'
  }
}

function sortPeople(items: PersonItem[]) {
  return [...items].sort((left, right) =>
    left.name.localeCompare(right.name) || left.id - right.id,
  )
}

async function loadPeople() {
  peopleLoading.value = true
  personError.value = ''
  try {
    const response = await fetchPeople()
    people.value = sortPeople(response.people)
    const selectedStillExists = response.people.some((person) => person.id === selectedPersonID.value)
    if (selectedPersonID.value && !selectedStillExists) {
      resetPersonDetail()
      window.history.replaceState({}, '', '/people')
    }
    if (selectedPersonID.value) {
      await loadPersonGallery(true)
    }
  } catch (error) {
    personError.value = error instanceof Error ? error.message : '模特加载失败'
  } finally {
    peopleLoading.value = false
  }
}

async function togglePersonFavorite(person: PersonItem) {
  if (favoriteSavingPersonIDs.value.has(person.id)) return
  favoriteSavingPersonIDs.value = new Set([...favoriteSavingPersonIDs.value, person.id])
  personError.value = ''
  const favorite = !person.favorite
  try {
    await updatePersonFavorite(person.id, favorite)
    people.value = sortPeople(people.value.map((item) =>
      item.id === person.id ? { ...item, favorite } : item,
    ))
  } catch (error) {
    personError.value = error instanceof Error ? error.message : '更新模特收藏失败'
  } finally {
    const next = new Set(favoriteSavingPersonIDs.value)
    next.delete(person.id)
    favoriteSavingPersonIDs.value = next
  }
}

function updateAlbumPeople(
  groupsToUpdate: PhotoGroup[],
  albumID: number,
  update: (people: PhotoGroup['people']) => PhotoGroup['people'],
) {
  return groupsToUpdate.map((group) =>
    group.albumID === albumID ? { ...group, people: update(group.people) } : group,
  )
}

async function openAlbumPersonPicker(group: PhotoGroup) {
  albumPersonTarget.value = group
  albumPersonSearch.value = ''
  albumPersonError.value = ''
  albumPersonDialog.value?.showModal()
  if (people.value.length || albumPersonLoading.value) return

  albumPersonLoading.value = true
  try {
    const response = await fetchPeople()
    people.value = sortPeople(response.people)
  } catch (error) {
    albumPersonError.value = error instanceof Error ? error.message : '模特列表加载失败'
  } finally {
    albumPersonLoading.value = false
  }
}

function closeAlbumPersonPicker() {
  if (personAlbumSavingKeys.value.size) return
  albumPersonDialog.value?.close()
}

function resetAlbumPersonPicker() {
  albumPersonTarget.value = null
  albumPersonSearch.value = ''
  albumPersonError.value = ''
}

function closeAlbumPersonPickerFromBackdrop(event: MouseEvent) {
  if (event.target === event.currentTarget) closeAlbumPersonPicker()
}

async function associateAlbumPerson(person: PersonItem) {
  const group = albumPersonTarget.value
  if (!group) return
  const key = `${person.id}:${group.albumID}`
  if (personAlbumSavingKeys.value.has(key)) return
  personAlbumSavingKeys.value = new Set([...personAlbumSavingKeys.value, key])
  albumPersonError.value = ''
  try {
    const response = await addPersonAlbum(person.id, group.albumID)
    const relatedPerson = { id: person.id, name: person.name, note: person.note }
    const appendPerson = (items: PhotoGroup['people']) =>
      items.some((item) => item.id === person.id) ? items : [...items, relatedPerson]
    groups.value = updateAlbumPeople(groups.value, group.albumID, appendPerson)
    personGroups.value = updateAlbumPeople(personGroups.value, group.albumID, appendPerson)
    seriesGroups.value = updateAlbumPeople(seriesGroups.value, group.albumID, appendPerson)
    favoriteGroups.value = updateAlbumPeople(favoriteGroups.value, group.albumID, appendPerson)
    people.value = sortPeople(people.value.map((item) =>
      item.id === person.id ? response.person : item,
    ))
    albumPersonDialog.value?.close()
  } catch (error) {
    albumPersonError.value = error instanceof Error ? error.message : '关联模特失败'
  } finally {
    const next = new Set(personAlbumSavingKeys.value)
    next.delete(key)
    personAlbumSavingKeys.value = next
  }
}

async function detachAlbumPerson(personID: number, group: PhotoGroup) {
  const key = `${personID}:${group.albumID}`
  if (personAlbumSavingKeys.value.has(key)) return
  const personName = group.people.find((person) => person.id === personID)?.name ?? '该模特'
  const confirmed = await requestConfirmation({
    title: '移除关联相册',
    message: `确定要从“${personName}”中移除相册“${group.name}”吗？`,
    confirmLabel: '移除关联',
  })
  if (!confirmed) return
  personAlbumSavingKeys.value = new Set([...personAlbumSavingKeys.value, key])
  personError.value = ''
  try {
    const response = await removePersonAlbum(personID, group.albumID)
    const removePerson = (items: PhotoGroup['people']) =>
      items.filter((person) => person.id !== personID)
    groups.value = updateAlbumPeople(groups.value, group.albumID, removePerson)
    favoriteGroups.value = updateAlbumPeople(favoriteGroups.value, group.albumID, removePerson)
    seriesGroups.value = updateAlbumPeople(seriesGroups.value, group.albumID, removePerson)
    if (selectedPersonID.value === personID) {
      personGroups.value = personGroups.value.filter((item) => item.albumID !== group.albumID)
      personTotalMedia.value = Math.max(0, personTotalMedia.value - group.count)
    } else {
      personGroups.value = updateAlbumPeople(personGroups.value, group.albumID, removePerson)
    }
    people.value = sortPeople(people.value.map((person) =>
      person.id === personID ? response.person : person,
    ))
    await nextTick()
    maybeLoadMore()
  } catch (error) {
    personError.value = error instanceof Error ? error.message : '移除模特相册失败'
  } finally {
    const next = new Set(personAlbumSavingKeys.value)
    next.delete(key)
    personAlbumSavingKeys.value = next
  }
}

function openPersonCreateDialog() {
  personError.value = ''
  personCreateDialog.value?.showModal()
}

function resetPersonCreateForm() {
  personName.value = ''
  personNote.value = ''
  personAliases.value = ''
  newPersonAlbumOptions.value = []
  newPersonSelectedAlbumIDs.value = new Set()
  personError.value = ''
}

function closePersonCreateDialog() {
  if (personSaving.value) return
  personCreateDialog.value?.close()
}

function closePersonCreateFromBackdrop(event: MouseEvent) {
  if (event.target === event.currentTarget) closePersonCreateDialog()
}

async function addPerson() {
  const name = personName.value.trim()
  if (!name || personSaving.value) return
  personSaving.value = true
  personError.value = ''
  try {
    const response = await createPerson(
      name,
      [...newPersonSelectedAlbumIDs.value],
      personNote.value.trim(),
      parsePersonAliases(personAliases.value),
    )
    personName.value = ''
    personNote.value = ''
    personAliases.value = ''
    newPersonAlbumOptions.value = []
    newPersonSelectedAlbumIDs.value = new Set()
    people.value = sortPeople([
      ...people.value.filter((person) => person.id !== response.person.id),
      response.person,
    ])
    personCreateDialog.value?.close()
    await openPerson(response.person.id)
  } catch (error) {
    personError.value = error instanceof Error ? error.message : '添加模特失败'
  } finally {
    personSaving.value = false
  }
}

function openPersonEditDialog() {
  const person = selectedPerson.value
  if (!person || personSaving.value) return
  personEditName.value = person.name
  editPersonAlbumSearch.value = person.name
  personEditNote.value = person.note
  personEditAliases.value = person.aliases.join(', ')
  editingPersonID.value = person.id
  personError.value = ''
  personEditDialog.value?.showModal()
}

function resetPersonEditForm() {
  ++editPersonAlbumSearchVersion
  editPersonAlbumLoading.value = false
  editPersonLastSearch.value = null
  editPersonRemovedAlbumIDs.value = new Set()
  editingPersonID.value = null
  editPersonSelectedAlbumIDs.value = new Set()
  editPersonAlbumOptions.value = []
  editPersonAlbumError.value = ''
  personEditName.value = ''
  editPersonAlbumSearch.value = ''
  personEditNote.value = ''
  personEditAliases.value = ''
  personError.value = ''
}

function closePersonEditDialog() {
  if (personSaving.value) return
  personEditDialog.value?.close()
}

function closePersonEditFromBackdrop(event: MouseEvent) {
  if (event.target === event.currentTarget) closePersonEditDialog()
}

async function searchEditPersonAlbums() {
  const id = editingPersonID.value
  if (id === null || personSaving.value) return
  const search = editPersonAlbumSearch.value.trim()
  const version = ++editPersonAlbumSearchVersion
  editPersonAlbumOptions.value = []
  editPersonAlbumError.value = ''
  editPersonLastSearch.value = search
  editPersonAlbumLoading.value = true
  try {
    const response = await fetchPersonAlbumOptions(id, search)
    if (version !== editPersonAlbumSearchVersion) return
    editPersonAlbumOptions.value = response.albums
  } catch (error) {
    if (version !== editPersonAlbumSearchVersion) return
    editPersonAlbumError.value = error instanceof Error ? error.message : '搜索相册失败'
  } finally {
    if (version === editPersonAlbumSearchVersion) editPersonAlbumLoading.value = false
  }
}

function editPersonAlbumSelected(album: PersonAlbumOption) {
  return album.associated ? !editPersonRemovedAlbumIDs.value.has(album.id) : editPersonSelectedAlbumIDs.value.has(album.id)
}

function setEditPersonAlbumSelected(album: PersonAlbumOption, selected: boolean) {
  const added = new Set(editPersonSelectedAlbumIDs.value)
  const removed = new Set(editPersonRemovedAlbumIDs.value)
  if (album.associated) {
    added.delete(album.id)
    if (selected) removed.delete(album.id)
    else removed.add(album.id)
  } else {
    removed.delete(album.id)
    if (selected) added.add(album.id)
    else added.delete(album.id)
  }
  editPersonSelectedAlbumIDs.value = added
  editPersonRemovedAlbumIDs.value = removed
}

function toggleEditPersonAlbum(album: PersonAlbumOption) {
  setEditPersonAlbumSelected(album, !editPersonAlbumSelected(album))
}

function toggleAllEditPersonAlbums() {
  const selected = !allEditPersonAlbumsSelected.value
  for (const album of editPersonAlbumOptions.value) setEditPersonAlbumSelected(album, selected)
}

async function saveSelectedPerson() {
  const person = selectedPerson.value
  const name = personEditName.value.trim()
  if (!person || personSaving.value) return
  if (!name) {
    personError.value = '模特名称不能为空'
    return
  }
  personSaving.value = true
  personError.value = ''
  const albumIDs = [...editPersonSelectedAlbumIDs.value]
  const removeAlbumIDs = [...editPersonRemovedAlbumIDs.value]
  try {
    const response = await updatePerson(person.id, {
      name,
      note: personEditNote.value.trim(),
      aliases: parsePersonAliases(personEditAliases.value),
      album_ids: albumIDs,
      remove_album_ids: removeAlbumIDs,
    })
    people.value = sortPeople(people.value.map((item) =>
      item.id === person.id ? response.person : item,
    ))
    const updateRelatedPerson = (items: PhotoGroup['people']) => {
      const withoutPerson = items.filter(item => item.id !== person.id)
      return [...withoutPerson, { id: response.person.id, name: response.person.name, note: response.person.note }]
    }
    const updateGroups = (items: PhotoGroup[]) => items.map(group => {
      if (removeAlbumIDs.includes(group.albumID)) return { ...group, people: group.people.filter(item => item.id !== person.id) }
      return albumIDs.includes(group.albumID) || group.people.some(item => item.id === person.id)
        ? { ...group, people: updateRelatedPerson(group.people) } : group
    })
    groups.value = updateGroups(groups.value)
    favoriteGroups.value = updateGroups(favoriteGroups.value)
    seriesGroups.value = updateGroups(seriesGroups.value)
    personGroups.value = updateGroups(personGroups.value)
    personEditDialog.value?.close()
    if (albumIDs.length || removeAlbumIDs.length) await loadPersonGallery(true)
  } catch (error) {
    personError.value = error instanceof Error ? error.message : '更新模特信息失败'
  } finally {
    personSaving.value = false
  }
}

async function removeSelectedPerson() {
  const person = selectedPerson.value
  if (!person || personSaving.value) return
  const confirmed = await requestConfirmation({
    title: '删除模特',
    message: `确定删除模特“${person.name}”吗？\n只会删除模特及其相册关联，不会删除相册或媒体文件。`,
    confirmLabel: '删除模特',
  })
  if (!confirmed) return

  personSaving.value = true
  personError.value = ''
  try {
    await deletePerson(person.id)
    people.value = people.value.filter((item) => item.id !== person.id)
    const removePerson = (items: PhotoGroup['people']) =>
      items.filter((item) => item.id !== person.id)
    groups.value = groups.value.map((group) => ({
      ...group,
      people: removePerson(group.people),
    }))
    favoriteGroups.value = favoriteGroups.value.map((group) => ({
      ...group,
      people: removePerson(group.people),
    }))
    seriesGroups.value = seriesGroups.value.map((group) => ({
      ...group,
      people: removePerson(group.people),
    }))
    personEditDialog.value?.close()
    await showPeopleList()
  } catch (error) {
    personError.value = error instanceof Error ? error.message : '删除模特失败'
  } finally {
    personSaving.value = false
  }
}

async function loadNewPersonAlbumOptions() {
  const name = personName.value.trim()
  if (!name) {
    newPersonAlbumOptions.value = []
    return
  }
  newPersonAlbumSearchLoading.value = true
  personError.value = ''
  try {
    const response = await searchAlbums(name)
    if (personName.value.trim() === name) newPersonAlbumOptions.value = response.albums
  } catch (error) {
    personError.value = error instanceof Error ? error.message : '匹配模特相册失败'
  } finally {
    newPersonAlbumSearchLoading.value = false
  }
}

function toggleNewPersonAlbum(albumID: number) {
  const next = new Set(newPersonSelectedAlbumIDs.value)
  next.has(albumID) ? next.delete(albumID) : next.add(albumID)
  newPersonSelectedAlbumIDs.value = next
}

function toggleAllNewPersonAlbums() {
  const next = new Set(newPersonSelectedAlbumIDs.value)
  for (const album of newPersonAlbumOptions.value) {
    if (allNewPersonAlbumsSelected.value) {
      next.delete(album.id)
    } else {
      next.add(album.id)
    }
  }
  newPersonSelectedAlbumIDs.value = next
}

function resetPersonDetail() {
  selectedPersonID.value = null
  personGroups.value = []
  personTotalMedia.value = 0
  personHasMoreAlbums.value = false
}

async function showPeopleList(updateHistory = true) {
  activeView.value = 'people'
  resetSeriesDetail()
  resetPersonDetail()
  if (updateHistory && window.location.pathname !== '/people') {
    window.history.pushState({}, '', '/people')
  }
  if (content.value) content.value.scrollTop = 0
  await loadPeople()
}

async function openPerson(personID: number, updateHistory = true) {
  activeView.value = 'people'
  resetSeriesDetail()
  selectedPersonID.value = personID
  personGroups.value = []
  personTotalMedia.value = 0
  personHasMoreAlbums.value = false
  const path = `/people/${personID}`
  if (updateHistory && window.location.pathname !== path) window.history.pushState({}, '', path)
  if (content.value) content.value.scrollTop = 0
  if (!people.value.some((person) => person.id === personID)) {
    await loadPeople()
    return
  }
  await loadPersonGallery(true)
}

function handlePopState() {
  if (window.location.pathname === '/trash' || window.location.pathname === '/trash/') {
    window.history.replaceState({}, '', '/')
  }
  const match = window.location.pathname.match(/^\/people\/(\d+)\/?$/)
  if (match) {
    void openPerson(Number(match[1]), false)
    return
  }
  if (window.location.pathname === '/people' || window.location.pathname === '/people/') {
    void showPeopleList(false)
    return
  }
  const seriesMatch = window.location.pathname.match(/^\/series\/(\d+)\/?$/)
  if (seriesMatch) {
    void openSeries(Number(seriesMatch[1]), false)
    return
  }
  if (window.location.pathname === '/series' || window.location.pathname === '/series/') {
    void showSeriesList(false)
    return
  }
  const view = viewFromPath(window.location.pathname)
  if (view === 'photos') {
    const wasPhotos = activeView.value === 'photos'
    const search = new URLSearchParams(window.location.search).get('q')?.trim() ?? ''
    const changed = search !== gallerySearch.value
    gallerySearch.value = search
    gallerySearchInput.value = search
    setView('photos', false)
    if (changed && wasPhotos) void loadGallery(true)
    return
  }
  setView(view, false)
}

async function loadPersonGallery(reset = false) {
  const personID = selectedPersonID.value
  if (!personID || personGalleryLoadingMore.value || (!reset && !personHasMoreAlbums.value)) return
  if (reset) personGalleryLoading.value = true
  else personGalleryLoadingMore.value = true
  personError.value = ''
  try {
    const offset = reset ? 0 : personGroups.value.length
    const response = await fetchPersonGallery(personID, offset, reset ? 5 : 10)
    if (selectedPersonID.value !== personID) return
    const incoming = response.albums.map(toGroup)
    personGroups.value = reset ? incoming : [...personGroups.value, ...incoming]
    personTotalMedia.value = response.total_media
    personHasMoreAlbums.value = response.pagination.has_more
    people.value = people.value.map((person) =>
      person.id === personID ? response.person : person,
    )
  } catch (error) {
    personError.value = error instanceof Error ? error.message : '模特相册加载失败'
  } finally {
    personGalleryLoading.value = false
    personGalleryLoadingMore.value = false
    await nextTick()
    maybeLoadMore()
  }
}

function sortSeries(items: SeriesItem[]) {
  return [...items].sort((left, right) =>
    left.name.localeCompare(right.name) || left.id - right.id,
  )
}

async function loadSeries() {
  seriesLoading.value = true
  seriesError.value = ''
  try {
    const response = await fetchSeries()
    series.value = sortSeries(response.series)
    const selectedStillExists = response.series.some((item) => item.id === selectedSeriesID.value)
    if (selectedSeriesID.value && !selectedStillExists) {
      resetSeriesDetail()
      window.history.replaceState({}, '', '/series')
    }
    if (selectedSeriesID.value) await loadSeriesGallery(true)
  } catch (error) {
    seriesError.value = error instanceof Error ? error.message : '系列加载失败'
  } finally {
    seriesLoading.value = false
  }
}

async function toggleSeriesFavorite(item: SeriesItem) {
  if (favoriteSavingSeriesIDs.value.has(item.id)) return
  favoriteSavingSeriesIDs.value = new Set([...favoriteSavingSeriesIDs.value, item.id])
  seriesError.value = ''
  try {
    const response = await updateSeries(item.id, { favorite: !item.favorite })
    series.value = sortSeries(series.value.map((current) =>
      current.id === item.id ? response.series : current,
    ))
  } catch (error) {
    seriesError.value = error instanceof Error ? error.message : '更新系列收藏失败'
  } finally {
    const next = new Set(favoriteSavingSeriesIDs.value)
    next.delete(item.id)
    favoriteSavingSeriesIDs.value = next
  }
}

function openSeriesCreateDialog() {
  seriesError.value = ''
  seriesCreateDialog.value?.showModal()
}

function resetSeriesCreateForm() {
  seriesName.value = ''
  newSeriesAlbumOptions.value = []
  newSeriesSelectedAlbumIDs.value = new Set()
  seriesError.value = ''
}

function closeSeriesCreateDialog() {
  if (!seriesSaving.value) seriesCreateDialog.value?.close()
}

function closeSeriesCreateFromBackdrop(event: MouseEvent) {
  if (event.target === event.currentTarget) closeSeriesCreateDialog()
}

async function addSeries() {
  const name = seriesName.value.trim()
  if (!name || seriesSaving.value) return
  seriesSaving.value = true
  seriesError.value = ''
  try {
    const response = await createSeries(name, [...newSeriesSelectedAlbumIDs.value])
    series.value = sortSeries([
      ...series.value.filter((item) => item.id !== response.series.id),
      response.series,
    ])
    seriesCreateDialog.value?.close()
    await openSeries(response.series.id)
  } catch (error) {
    seriesError.value = error instanceof Error ? error.message : '添加系列失败'
  } finally {
    seriesSaving.value = false
  }
}

async function loadNewSeriesAlbumOptions() {
  const name = seriesName.value.trim()
  if (!name) {
    newSeriesAlbumOptions.value = []
    return
  }
  newSeriesAlbumSearchLoading.value = true
  seriesError.value = ''
  try {
    const response = await searchAlbums(name)
    if (seriesName.value.trim() === name) newSeriesAlbumOptions.value = response.albums
  } catch (error) {
    seriesError.value = error instanceof Error ? error.message : '匹配系列相册失败'
  } finally {
    newSeriesAlbumSearchLoading.value = false
  }
}

function toggleNewSeriesAlbum(albumID: number) {
  const next = new Set(newSeriesSelectedAlbumIDs.value)
  next.has(albumID) ? next.delete(albumID) : next.add(albumID)
  newSeriesSelectedAlbumIDs.value = next
}

function toggleAllNewSeriesAlbums() {
  const next = new Set(newSeriesSelectedAlbumIDs.value)
  for (const album of newSeriesAlbumOptions.value) {
    if (allNewSeriesAlbumsSelected.value) next.delete(album.id)
    else next.add(album.id)
  }
  newSeriesSelectedAlbumIDs.value = next
}

function resetSeriesDetail() {
  selectedSeriesID.value = null
  seriesGroups.value = []
  seriesTotalMedia.value = 0
  seriesHasMoreAlbums.value = false
}

async function showSeriesList(updateHistory = true) {
  activeView.value = 'series'
  resetPersonDetail()
  resetSeriesDetail()
  if (updateHistory && window.location.pathname !== '/series') {
    window.history.pushState({}, '', '/series')
  }
  if (content.value) content.value.scrollTop = 0
  await loadSeries()
}

async function openSeries(seriesID: number, updateHistory = true) {
  activeView.value = 'series'
  resetPersonDetail()
  selectedSeriesID.value = seriesID
  seriesGroups.value = []
  seriesTotalMedia.value = 0
  seriesHasMoreAlbums.value = false
  const path = `/series/${seriesID}`
  if (updateHistory && window.location.pathname !== path) window.history.pushState({}, '', path)
  if (content.value) content.value.scrollTop = 0
  if (!series.value.some((item) => item.id === seriesID)) {
    await loadSeries()
    return
  }
  await loadSeriesGallery(true)
}

async function loadSeriesGallery(reset = false) {
  const seriesID = selectedSeriesID.value
  if (!seriesID || seriesGalleryLoadingMore.value || (!reset && !seriesHasMoreAlbums.value)) return
  if (reset) seriesGalleryLoading.value = true
  else seriesGalleryLoadingMore.value = true
  seriesError.value = ''
  try {
    const offset = reset ? 0 : seriesGroups.value.length
    const response = await fetchSeriesGallery(seriesID, offset, reset ? 5 : 10)
    if (selectedSeriesID.value !== seriesID) return
    const incoming = response.albums.map(toGroup)
    seriesGroups.value = reset ? incoming : [...seriesGroups.value, ...incoming]
    seriesTotalMedia.value = response.total_media
    seriesHasMoreAlbums.value = response.pagination.has_more
    series.value = series.value.map((item) => item.id === seriesID ? response.series : item)
  } catch (error) {
    seriesError.value = error instanceof Error ? error.message : '系列相册加载失败'
  } finally {
    seriesGalleryLoading.value = false
    seriesGalleryLoadingMore.value = false
    await nextTick()
    maybeLoadMore()
  }
}

function openSeriesEditDialog() {
  const item = selectedSeries.value
  if (!item || seriesSaving.value) return
  seriesEditName.value = item.name
  editSeriesAlbumSearch.value = item.name
  editingSeriesID.value = item.id
  seriesError.value = ''
  seriesEditDialog.value?.showModal()
}

function resetSeriesEditForm() {
  ++editSeriesAlbumSearchVersion
  editSeriesAlbumLoading.value = false
  editSeriesLastSearch.value = null
  editSeriesRemovedAlbumIDs.value = new Set()
  editingSeriesID.value = null
  editSeriesSelectedAlbumIDs.value = new Set()
  editSeriesAlbumOptions.value = []
  editSeriesAlbumError.value = ''
  editSeriesAlbumSearch.value = ''
  editSeriesAlbumRegex.value = false
  seriesEditName.value = ''
  seriesError.value = ''
}

function closeSeriesEditDialog() {
  if (seriesSaving.value) return
  seriesEditDialog.value?.close()
}

function closeSeriesEditFromBackdrop(event: MouseEvent) {
  if (event.target === event.currentTarget) closeSeriesEditDialog()
}

async function searchEditSeriesAlbums() {
  const id = editingSeriesID.value
  if (id === null || seriesSaving.value) return
  const search = editSeriesAlbumSearch.value.trim()
  const version = ++editSeriesAlbumSearchVersion
  editSeriesAlbumOptions.value = []
  editSeriesAlbumError.value = ''
  editSeriesLastSearch.value = search
  editSeriesAlbumLoading.value = true
  try {
    const response = await fetchSeriesAlbumOptions(id, search, editSeriesAlbumRegex.value)
    if (version !== editSeriesAlbumSearchVersion) return
    editSeriesAlbumOptions.value = response.albums
  } catch (error) {
    if (version !== editSeriesAlbumSearchVersion) return
    editSeriesAlbumError.value = error instanceof Error ? error.message : '搜索相册失败'
  } finally {
    if (version === editSeriesAlbumSearchVersion) editSeriesAlbumLoading.value = false
  }
}

function editSeriesAlbumSelected(album: PersonAlbumOption) {
  return album.associated ? !editSeriesRemovedAlbumIDs.value.has(album.id) : editSeriesSelectedAlbumIDs.value.has(album.id)
}

function setEditSeriesAlbumSelected(album: PersonAlbumOption, selected: boolean) {
  const added = new Set(editSeriesSelectedAlbumIDs.value)
  const removed = new Set(editSeriesRemovedAlbumIDs.value)
  if (album.associated) {
    added.delete(album.id)
    if (selected) removed.delete(album.id)
    else removed.add(album.id)
  } else {
    removed.delete(album.id)
    if (selected) added.add(album.id)
    else added.delete(album.id)
  }
  editSeriesSelectedAlbumIDs.value = added
  editSeriesRemovedAlbumIDs.value = removed
}

function toggleEditSeriesAlbum(album: PersonAlbumOption) {
  setEditSeriesAlbumSelected(album, !editSeriesAlbumSelected(album))
}

function toggleAllEditSeriesAlbums() {
  const selected = !allEditSeriesAlbumsSelected.value
  for (const album of editSeriesAlbumOptions.value) setEditSeriesAlbumSelected(album, selected)
}

async function saveSelectedSeries() {
  const item = selectedSeries.value
  const name = seriesEditName.value.trim()
  if (!item || !name || seriesSaving.value) return
  if (name === item.name && !editSeriesSelectedAlbumIDs.value.size && !editSeriesRemovedAlbumIDs.value.size) {
    seriesEditDialog.value?.close()
    return
  }
  seriesSaving.value = true
  seriesError.value = ''
  const albumIDs = [...editSeriesSelectedAlbumIDs.value]
  const removeAlbumIDs = [...editSeriesRemovedAlbumIDs.value]
  try {
    const response = await updateSeries(item.id, { name, album_ids: albumIDs, remove_album_ids: removeAlbumIDs })
    series.value = sortSeries(series.value.map((current) =>
      current.id === item.id ? response.series : current,
    ))
    const updateGroups = (items: PhotoGroup[]) => items.map(group => {
      if (removeAlbumIDs.includes(group.albumID)) return { ...group, series: group.series.filter(related => related.id !== item.id) }
      if (albumIDs.includes(group.albumID)) return { ...group, series: [{ id: item.id, name: response.series.name }] }
      return { ...group, series: group.series.map(related => related.id === item.id ? { ...related, name: response.series.name } : related) }
    })
    groups.value = updateGroups(groups.value)
    favoriteGroups.value = updateGroups(favoriteGroups.value)
    personGroups.value = updateGroups(personGroups.value)
    seriesGroups.value = updateGroups(seriesGroups.value)
    seriesEditDialog.value?.close()
    if (albumIDs.length || removeAlbumIDs.length) await loadSeries()
  } catch (error) {
    seriesError.value = error instanceof Error ? error.message : '修改系列失败'
  } finally {
    seriesSaving.value = false
  }
}

async function removeSelectedSeries() {
  const item = selectedSeries.value
  if (!item || seriesSaving.value) return
  const confirmed = await requestConfirmation({
    title: '删除系列',
    message: `确定删除系列“${item.name}”吗？\n只会删除系列及其关联，不会删除相册或媒体文件。`,
    confirmLabel: '删除系列',
  })
  if (!confirmed) return
  seriesSaving.value = true
  seriesError.value = ''
  try {
    await deleteSeries(item.id)
    series.value = series.value.filter((current) => current.id !== item.id)
    await showSeriesList()
  } catch (error) {
    seriesError.value = error instanceof Error ? error.message : '删除系列失败'
  } finally {
    seriesSaving.value = false
  }
}

function updateAlbumSeries(
  groupsToUpdate: PhotoGroup[],
  albumID: number,
  update: (items: PhotoGroup['series']) => PhotoGroup['series'],
) {
  return groupsToUpdate.map((group) =>
    group.albumID === albumID ? { ...group, series: update(group.series) } : group,
  )
}

async function detachAlbumSeries(seriesID: number, group: PhotoGroup) {
  const key = `${seriesID}:${group.albumID}`
  if (seriesAlbumSavingKeys.value.has(key)) return
  const name = group.series.find((item) => item.id === seriesID)?.name ?? '该系列'
  const confirmed = await requestConfirmation({
    title: '移除关联相册',
    message: `确定要从“${name}”中移除相册“${group.name}”吗？`,
    confirmLabel: '移除关联',
  })
  if (!confirmed) return
  seriesAlbumSavingKeys.value = new Set([...seriesAlbumSavingKeys.value, key])
  seriesError.value = ''
  try {
    const response = await removeSeriesAlbum(seriesID, group.albumID)
    const remove = (items: PhotoGroup['series']) => items.filter((item) => item.id !== seriesID)
    groups.value = updateAlbumSeries(groups.value, group.albumID, remove)
    favoriteGroups.value = updateAlbumSeries(favoriteGroups.value, group.albumID, remove)
    personGroups.value = updateAlbumSeries(personGroups.value, group.albumID, remove)
    if (selectedSeriesID.value === seriesID) {
      seriesGroups.value = seriesGroups.value.filter((item) => item.albumID !== group.albumID)
      seriesTotalMedia.value = Math.max(0, seriesTotalMedia.value - group.count)
    } else {
      seriesGroups.value = updateAlbumSeries(seriesGroups.value, group.albumID, remove)
    }
    series.value = sortSeries(series.value.map((item) =>
      item.id === seriesID ? response.series : item,
    ))
    await nextTick()
    maybeLoadMore()
  } catch (error) {
    seriesError.value = error instanceof Error ? error.message : '移除系列相册失败'
  } finally {
    const next = new Set(seriesAlbumSavingKeys.value)
    next.delete(key)
    seriesAlbumSavingKeys.value = next
  }
}

function applyAlbumFavorite(groupsToUpdate: PhotoGroup[], albumID: number, favorite: boolean) {
  return groupsToUpdate.map((group) =>
    group.albumID === albumID ? { ...group, favorite } : group,
  )
}

function applyPhotoFavorite(groupsToUpdate: PhotoGroup[], photoID: string, favorite: boolean) {
  return groupsToUpdate.map((group) => {
    if (!group.photos.some((photo) => photo.id === photoID)) return group
    return {
      ...group,
      photos: group.photos.map((photo) =>
        photo.id === photoID ? { ...photo, favorite } : photo,
      ),
    }
  })
}

function createFavoritePhotoGroup(photos: Photo[]): PhotoGroup {
  return {
    id: 'favorite-photos',
    albumID: 0,
    name: '收藏的图片',
    detail: '单张图片收藏',
    count: photos.length,
    favorite: false,
    canFavorite: false,
    people: [],
    series: [],
    photos,
  }
}

async function setAlbumFavorite(albumID: number, currentFavorite: boolean, mediaCount: number) {
  if (favoriteSavingAlbumIDs.value.has(albumID)) return
  favoriteSavingAlbumIDs.value = new Set([...favoriteSavingAlbumIDs.value, albumID])
  const favorite = !currentFavorite
  if (activeView.value === 'favorites') favoriteGalleryError.value = ''
  else if (activeView.value === 'people') personError.value = ''
  else if (activeView.value === 'series') seriesError.value = ''
  else galleryError.value = ''

  try {
    await updateAlbumFavorite(albumID, favorite)
    groups.value = applyAlbumFavorite(groups.value, albumID, favorite)
    personGroups.value = applyAlbumFavorite(personGroups.value, albumID, favorite)
    seriesGroups.value = applyAlbumFavorite(seriesGroups.value, albumID, favorite)

    const existedInFavoriteGallery = favoriteGroups.value.some((group) => group.albumID === albumID)
    favoriteGroups.value = favorite
      ? applyAlbumFavorite(favoriteGroups.value, albumID, true)
      : favoriteGroups.value.filter((group) => group.albumID !== albumID)
    if (!favorite && existedInFavoriteGallery) {
      favoriteAlbumCount.value = Math.max(0, favoriteAlbumCount.value - 1)
      favoriteMediaCount.value = Math.max(0, favoriteMediaCount.value - mediaCount)
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : '更新相册收藏失败'
    if (activeView.value === 'favorites') favoriteGalleryError.value = message
    else if (activeView.value === 'people') personError.value = message
    else if (activeView.value === 'series') seriesError.value = message
    else galleryError.value = message
  } finally {
    const next = new Set(favoriteSavingAlbumIDs.value)
    next.delete(albumID)
    favoriteSavingAlbumIDs.value = next
  }
}

function toggleAlbumFavorite(group: PhotoGroup) {
  void setAlbumFavorite(group.albumID, group.favorite, group.count)
}

async function togglePhotoFavorite(photo: Photo) {
  if (favoriteSavingPhotoIDs.value.has(photo.id)) return
  const photoID = Number(photo.id)
  if (!Number.isSafeInteger(photoID) || photoID <= 0) return
  favoriteSavingPhotoIDs.value = new Set([...favoriteSavingPhotoIDs.value, photo.id])
  const favorite = !photo.favorite
  if (activeView.value === 'favorites') favoriteGalleryError.value = ''
  else if (activeView.value === 'people') personError.value = ''
  else if (activeView.value === 'series') seriesError.value = ''
  else galleryError.value = ''

  try {
    await updatePhotoFavorite(photoID, favorite)
    groups.value = applyPhotoFavorite(groups.value, photo.id, favorite)
    personGroups.value = applyPhotoFavorite(personGroups.value, photo.id, favorite)
    seriesGroups.value = applyPhotoFavorite(seriesGroups.value, photo.id, favorite)
    favoriteGroups.value = applyPhotoFavorite(favoriteGroups.value, photo.id, favorite)
    previewPhotos.value = previewPhotos.value.map((item) =>
      item.id === photo.id ? { ...item, favorite } : item,
    )

    const favoritePhotos = favoritePhotoGroup.value?.photos ?? []
    const existedInPhotoFavorites = favoritePhotos.some((item) => item.id === photo.id)
    if (favorite) {
      const updatedPhoto = { ...photo, favorite: true }
      const updatedPhotos = existedInPhotoFavorites
        ? favoritePhotos.map((item) => item.id === photo.id ? updatedPhoto : item)
        : [updatedPhoto, ...favoritePhotos]
      favoritePhotoGroup.value = createFavoritePhotoGroup(updatedPhotos)
      if (!existedInPhotoFavorites) favoritePhotoCount.value += 1
    } else if (existedInPhotoFavorites) {
      const updatedPhotos = favoritePhotos.filter((item) => item.id !== photo.id)
      favoritePhotoGroup.value = updatedPhotos.length ? createFavoritePhotoGroup(updatedPhotos) : null
      favoritePhotoCount.value = Math.max(0, favoritePhotoCount.value - 1)
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : '更新图片收藏失败'
    if (activeView.value === 'favorites') favoriteGalleryError.value = message
    else if (activeView.value === 'people') personError.value = message
    else if (activeView.value === 'series') seriesError.value = message
    else galleryError.value = message
  } finally {
    const next = new Set(favoriteSavingPhotoIDs.value)
    next.delete(photo.id)
    favoriteSavingPhotoIDs.value = next
  }
}

function openPhoto(photo: Photo, group: PhotoGroup) {
  const completeGroup = visibleGroups.value.find((item) => item.id === group.id) ?? group
  previewPhotos.value = completeGroup.photos
  previewIndex.value = Math.max(0, completeGroup.photos.findIndex((item) => item.id === photo.id))
  lightboxLoadingPhotoID.value = photo.mediaType === 'image' ? photo.id : null
  slideshowPlaying.value = false
  lightbox.value?.showModal()
}

function clearLightboxPreloads() {
  for (const image of preloadedLightboxImages.values()) image.src = ''
  preloadedLightboxImages.clear()
}

function preloadUpcomingPhotos() {
  const photos = previewPhotos.value
  if (!lightbox.value?.open || photos.length < 2) {
    clearLightboxPreloads()
    return
  }

  const wanted = new Map<string, Photo>()
  // Keep the displayed image alive and include the previous slide as well.
  for (const offset of [0, -1]) {
    const photo = photos[(previewIndex.value + offset + photos.length) % photos.length]
    if (photo.mediaType === 'image') wanted.set(photo.id, photo)
  }
  const preloadCount = Math.min(lightboxPreloadCount, photos.length - 1)
  for (let offset = 1; offset <= preloadCount; offset += 1) {
    const photo = photos[(previewIndex.value + offset) % photos.length]
    if (photo.mediaType === 'image') wanted.set(photo.id, photo)
  }

  for (const [id, image] of preloadedLightboxImages) {
    if (wanted.has(id)) continue
    preloadedLightboxImages.delete(id)
    image.src = ''
  }

  for (const [id, photo] of wanted) {
    if (preloadedLightboxImages.has(id)) continue
    const image = new Image()
    image.decoding = 'async'
    image.fetchPriority = 'low'
    image.addEventListener(
      'error',
      () => {
        if (preloadedLightboxImages.get(id) === image) preloadedLightboxImages.delete(id)
      },
      { once: true },
    )
    preloadedLightboxImages.set(id, image)
    image.src = photo.original
  }
}

function markLightboxImageLoaded(photoID: string) {
  if (lightboxLoadingPhotoID.value === photoID) lightboxLoadingPhotoID.value = null
}

function markLightboxImageFailed(photoID: string) {
  if (lightboxLoadingPhotoID.value === photoID) lightboxLoadingPhotoID.value = null
}

async function closeLightbox() {
  if (lightboxClosing || !lightbox.value?.open) return
  lightboxClosing = true
  lightboxSwitchGeneration += 1
  slideshowPlaying.value = false
  window.clearTimeout(slideshowTimer)
  const dialog = lightbox.value
  dialog.classList.add('is-closing')
  // Keep the dialog in the top layer until its mask and content have faded out.
  const maskAnimations = dialog.getAnimations().map(animation => animation.finished.catch(() => {}))
  await Promise.all([lightboxSlides.value?.fadeOut(), ...maskAnimations])
  dialog.close()
}

function handleLightboxClose() {
  lightboxZoomed.value = false
  lightbox.value?.classList.remove('is-closing')
  lightboxClosing = false
  lightboxLoadingPhotoID.value = null
  lightboxSwitchGeneration += 1
  lightboxSwitchPending = false
  queuedLightboxDirection = 0
  slideshowPlaying.value = false
  window.clearTimeout(slideshowTimer)
  wheelLastSwitchAt = Number.NEGATIVE_INFINITY
  clearLightboxPreloads()
  previewPhotos.value = []
  previewIndex.value = 0
}

async function stepPhoto(direction: number) {
  if (lightboxClosing || previewPhotos.value.length < 2 || !lightbox.value?.open) return
  const step = direction > 0 ? 1 : -1
  if (lightboxSwitchPending) {
    // Coalesce repeated input instead of mounting several moving full-size images.
    queuedLightboxDirection = step
    return
  }
  lightboxSwitchPending = true
  const generation = ++lightboxSwitchGeneration
  const nextIndex = (previewIndex.value + step + previewPhotos.value.length) % previewPhotos.value.length
  const photo = previewPhotos.value[nextIndex]
  if (photo.mediaType === 'image' && !lightboxSlides.value?.isPhotoReady(photo.id)) {
    lightboxLoadingPhotoID.value = photo.id
    let image = preloadedLightboxImages.get(photo.id)
    if (!image) {
      image = new Image()
      image.decoding = 'async'
      image.src = photo.original
      preloadedLightboxImages.set(photo.id, image)
    }
    image.fetchPriority = 'high'
    try {
      // Network completion alone does not mean a large original is ready to paint.
      await image.decode()
    } catch {
      // Preserve navigation when an original is unavailable.
    }
  }
  if (generation !== lightboxSwitchGeneration || !lightbox.value?.open) return
  lightboxLoadingPhotoID.value = null
  try {
    await lightboxSlides.value?.slide(step)
    if (generation !== lightboxSwitchGeneration || !lightbox.value?.open) return
    previewIndex.value = nextIndex
    await nextTick()
  } finally {
    if (generation === lightboxSwitchGeneration) {
      lightboxSlides.value?.resetNavigation()
      finishLightboxSlide()
    }
  }
}

function finishLightboxSlide() {
  lightboxSwitchPending = false
  const direction = queuedLightboxDirection
  queuedLightboxDirection = 0
  if (direction) void stepPhoto(direction)
}

function scheduleSlideshow() {
  window.clearTimeout(slideshowTimer)
  if (
    !slideshowPlaying.value ||
    document.hidden ||
    !lightbox.value?.open ||
    currentPhoto.value?.mediaType === 'video'
  ) return

  slideshowProgressKey.value += 1
  slideshowTimer = window.setTimeout(() => stepPhoto(1), slideshowDelay)
}

function toggleSlideshow() {
  slideshowPlaying.value = !slideshowPlaying.value
}

function handleVideoEnded() {
  if (slideshowPlaying.value) stepPhoto(1)
}

function handleVisibilityChange() {
  scheduleSlideshow()
  if (document.visibilityState === 'visible' && authState.value === 'authenticated') {
    void loadDirectories(true)
  } else {
    window.clearTimeout(directoryRefreshTimer)
  }
}

function handleLightboxWheel(event: WheelEvent) {
  if (matchesWheelZoom(event, wheelZoomModifier.value)) {
    modifierShortcutChorded = true
    lightboxSlides.value?.zoomWithWheel(event)
    return
  }
  if (lightboxZoomed.value) return
  const rawDelta = Math.abs(event.deltaY) >= Math.abs(event.deltaX) ? event.deltaY : event.deltaX
  const deltaUnit = event.deltaMode === WheelEvent.DOM_DELTA_LINE
    ? 16
    : event.deltaMode === WheelEvent.DOM_DELTA_PAGE
      ? window.innerHeight
      : 1
  const delta = rawDelta * deltaUnit
  if (Math.abs(delta) < 1) return

  const now = performance.now()
  if (now - wheelLastSwitchAt < 500) return
  wheelLastSwitchAt = now
  stepPhoto(delta > 0 ? 1 : -1)
}

function closeFromBackdrop(event: MouseEvent) {
  if (event.target === event.currentTarget) closeLightbox()
}

function toPhoto(media: MediaItem): Photo {
  const fallbackRatio = media.media_type === 'video' ? 16 / 9 : 2 / 3
  const aspectRatio = media.width_px && media.height_px ? media.width_px / media.height_px : fallbackRatio
  return {
    id: String(media.id),
    name: media.name,
    favorite: media.favorite,
    thumbnail: media.thumbnail_url,
    original: media.original_url,
    mediaType: media.media_type,
    aspectRatio: Number.isFinite(aspectRatio) && aspectRatio > 0 ? aspectRatio : fallbackRatio,
  }
}

function toGroup(album: AlbumItem): PhotoGroup {
  return {
    id: `album-${album.id}`,
    albumID: album.id,
    publishedDate: album.published_date,
    name: album.name,
    detail: '照片与视频',
    count: album.media_count,
    favorite: album.favorite,
    canFavorite: true,
    people: album.people ?? [],
    series: album.series ?? [],
    photos: album.media.map(toPhoto),
  }
}

async function loadGallery(reset = false) {
  if (!reset && (galleryLoadingMore.value || galleryLoading.value || !hasMoreAlbums.value)) return

  const version = reset ? ++galleryLoadVersion : galleryLoadVersion
  const search = gallerySearch.value

  if (reset) {
    galleryLoading.value = true
  } else {
    galleryLoadingMore.value = true
  }
  galleryError.value = ''

  try {
    const offset = reset ? 0 : groups.value.length
    const response = await fetchGallery(offset, reset ? 5 : 10, false, search)
    if (version !== galleryLoadVersion || search !== gallerySearch.value) return
    const incoming = response.albums.map(toGroup)
    groups.value = reset ? incoming : [...groups.value, ...incoming]
    totalAlbumCount.value = response.pagination.total
    totalMediaCount.value = response.total_media
    hasMoreAlbums.value = response.pagination.has_more
  } catch (error) {
    if (version !== galleryLoadVersion) return
    galleryError.value = error instanceof Error ? error.message : '图库加载失败'
  } finally {
    if (version !== galleryLoadVersion) return
    galleryLoading.value = false
    galleryLoadingMore.value = false
    await nextTick()
    maybeLoadMore()
  }
}

async function loadFavoriteGallery(reset = false) {
  if (
    favoriteGalleryLoadingMore.value
    || (!reset && (favoriteGalleryLoading.value || !favoriteHasMoreAlbums.value))
  ) return

  if (reset) favoriteGalleryLoading.value = true
  else favoriteGalleryLoadingMore.value = true
  favoriteGalleryError.value = ''

  try {
    const offset = reset ? 0 : favoriteGroups.value.length
    const [response, photoGroup] = await Promise.all([
      fetchGallery(offset, reset ? 5 : 10, true),
      reset ? loadFavoritePhotoGroup() : Promise.resolve(undefined),
    ])
    const incoming = response.albums.map(toGroup)
    favoriteGroups.value = reset ? incoming : [...favoriteGroups.value, ...incoming]
    if (reset && photoGroup !== undefined) {
      favoritePhotoGroup.value = photoGroup
      favoritePhotoCount.value = photoGroup?.count ?? 0
    }
    favoriteAlbumCount.value = response.pagination.total
    favoriteMediaCount.value = response.total_media
    favoriteHasMoreAlbums.value = response.pagination.has_more
  } catch (error) {
    favoriteGalleryError.value = error instanceof Error ? error.message : '收藏相册加载失败'
  } finally {
    favoriteGalleryLoading.value = false
    favoriteGalleryLoadingMore.value = false
    await nextTick()
    maybeLoadMore()
  }
}

async function loadFavoritePhotoGroup(): Promise<PhotoGroup | null> {
  const media: MediaItem[] = []
  let offset = 0
  let total = 0

  do {
    const response = await fetchFavoritePhotos(offset)
    total = response.pagination.total
    media.push(...response.media)
    offset += response.media.length
    if (!response.pagination.has_more || response.media.length === 0) break
  } while (offset < total)

  if (!media.length) return null
  return createFavoritePhotoGroup(media.map(toPhoto))
}

function maybeLoadMore() {
  const element = content.value
  if (!element) return
  const remaining = element.scrollHeight - element.scrollTop - element.clientHeight
  const preloadDistance = element.clientHeight * 2
  if (remaining >= preloadDistance) return
  if (activeView.value === 'photos' && hasMoreAlbums.value && !galleryLoading.value && !galleryLoadingMore.value) {
    void loadGallery(false)
  }
  if (
    activeView.value === 'favorites'
    && favoriteHasMoreAlbums.value
    && !favoriteGalleryLoading.value
    && !favoriteGalleryLoadingMore.value
  ) {
    void loadFavoriteGallery(false)
  }
  if (activeView.value === 'people' && personHasMoreAlbums.value && !personGalleryLoading.value && !personGalleryLoadingMore.value) {
    void loadPersonGallery(false)
  }
  if (activeView.value === 'series' && seriesHasMoreAlbums.value && !seriesGalleryLoading.value && !seriesGalleryLoadingMore.value) {
    void loadSeriesGallery(false)
  }
}

function toggleTheme() {
  theme.value = theme.value === 'light' ? 'dark' : 'light'
}

function selectThemeSeed(value: string) {
  const normalized = normalizeThemeSeed(value)
  if (!normalized) return
  themeSeedDraft.value = normalized
  themeSeed.value = normalized
  localStorage.setItem(THEME_SEED_STORAGE_KEY, normalized)
  applyMaterialTheme(normalized, theme.value)
}

function handleThemeColorInput(event: Event) {
  selectThemeSeed((event.currentTarget as HTMLInputElement).value)
}

function openMobileResource() {
  mobileMoreOpen.value = false
  setView('folders')
}

function openMobileSettings() {
  mobileMoreOpen.value = false
  setView('settings')
}

function toggleMobileTheme() {
  toggleTheme()
  mobileMoreOpen.value = false
}

async function submitAuthentication() {
  const username = authUsername.value.trim()
  if (!username || !authPassword.value || authSaving.value) return
  authError.value = ''
  if (authState.value === 'setup') {
    if (authPassword.value.length < 8) {
      authError.value = '密码至少需要 8 个字符'
      return
    }
    if (authPassword.value !== authPasswordConfirm.value) {
      authError.value = '两次输入的密码不一致'
      return
    }
  }
  authSaving.value = true
  try {
    if (authState.value === 'setup') await setupAccount(username, authPassword.value)
    else await login(username, authPassword.value)
    window.location.reload()
  } catch (error) {
    authError.value = error instanceof Error ? error.message : '登录失败'
  } finally {
    authSaving.value = false
  }
}

function handleUnauthorized() {
  if (authState.value !== 'authenticated') return
  closeLightbox()
  gallerySearchDialog.value?.close()
  personCreateDialog.value?.close()
  personEditDialog.value?.close()
  seriesCreateDialog.value?.close()
  seriesEditDialog.value?.close()
  albumPersonDialog.value?.close()
  finishConfirmation(false)
  authPassword.value = ''
  authError.value = '登录已过期，请重新登录'
  authState.value = 'login'
}

async function saveAccountSettings() {
  if (!accountCurrentPassword.value || accountSaving.value) return
  accountError.value = ''
  accountMessage.value = ''
  if (accountNewPassword.value && accountNewPassword.value.length < 8) {
    accountError.value = '新密码至少需要 8 个字符'
    return
  }
  if (accountNewPassword.value !== accountNewPasswordConfirm.value) {
    accountError.value = '两次输入的新密码不一致'
    return
  }
  accountSaving.value = true
  try {
    const response = await updateAccount(
      accountUsername.value.trim(),
      accountCurrentPassword.value,
      accountNewPassword.value,
    )
    authUsername.value = response.user.username
    accountUsername.value = response.user.username
    accountCurrentPassword.value = ''
    accountNewPassword.value = ''
    accountNewPasswordConfirm.value = ''
    accountMessage.value = '账户设置已保存，其他设备的登录会话已退出。'
  } catch (error) {
    accountError.value = error instanceof Error ? error.message : '保存账户设置失败'
  } finally {
    accountSaving.value = false
  }
}

async function signOut() {
  if (authSaving.value) return
  authSaving.value = true
  try {
    await logout()
  } finally {
    window.location.assign('/')
  }
}

function isTextEntryTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false
  return target.matches('input, textarea, select, [contenteditable="true"]')
    || target.closest('[contenteditable="true"]') !== null
}

function normalizeShortcutKey(key: string) {
  if (key === ' ') return 'Space'
  if (key === 'Esc') return 'Escape'
  if (key === 'OS') return 'Meta'
  return key.length === 1 ? key.toLocaleUpperCase() : key
}

function shortcutFromKeyboardEvent(event: KeyboardEvent) {
  const key = normalizeShortcutKey(event.key)
  if (!key || key === 'Unidentified' || key === 'Dead' || key === 'Process') return ''
  if (key === 'Control' || key === 'Alt' || key === 'Shift' || key === 'Meta') return key

  const modifiers: string[] = []
  if (event.ctrlKey) modifiers.push('Control')
  if (event.altKey) modifiers.push('Alt')
  if (event.shiftKey) modifiers.push('Shift')
  if (event.metaKey) modifiers.push('Meta')
  return [...modifiers, key].join('+')
}

function formatShortcutLabel(shortcut: string) {
  const labels: Record<string, string> = {
    Control: 'Ctrl',
    Alt: 'Alt',
    Shift: 'Shift',
    Meta: 'Meta',
    Space: '空格',
  }
  return shortcut.split('+').map((part) => labels[part] ?? part).join(' + ')
}

function isModifierOnlyShortcut(shortcut: string) {
  return shortcut === 'Control' || shortcut === 'Alt' || shortcut === 'Shift' || shortcut === 'Meta'
}

function isConfiguredModifierPressed(event: KeyboardEvent, shortcut: string) {
  if (shortcut === 'Control') return event.ctrlKey
  if (shortcut === 'Alt') return event.altKey
  if (shortcut === 'Shift') return event.shiftKey
  if (shortcut === 'Meta') return event.metaKey
  return false
}

function canOpenSearchFromShortcut(event: KeyboardEvent) {
  return authState.value === 'authenticated'
    && !isTextEntryTarget(event.target)
    && !document.querySelector('dialog[open]')
}

function captureSearchShortcut(event: KeyboardEvent) {
  event.stopPropagation()
  if (event.key === 'Tab') return
  event.preventDefault()
  if (event.key === 'Escape') {
    shortcutMessage.value = 'Esc 用于关闭界面，不能设为搜索快捷键。'
    return
  }
  if (event.key === '+') {
    shortcutMessage.value = '“+”不能单独用作快捷键，请选择其他按键。'
    return
  }
  const shortcut = shortcutFromKeyboardEvent(event)
  if (!shortcut) return
  searchShortcutDraft.value = shortcut
  shortcutMessage.value = ''
}

function saveSearchShortcut() {
  searchShortcut.value = searchShortcutDraft.value || defaultSearchShortcut
  localStorage.setItem(searchShortcutStorageKey, searchShortcut.value)
  shortcutMessage.value = `搜索快捷键已保存为 ${formatShortcutLabel(searchShortcut.value)}。`
}

function resetSearchShortcut() {
  searchShortcut.value = defaultSearchShortcut
  searchShortcutDraft.value = defaultSearchShortcut
  localStorage.removeItem(searchShortcutStorageKey)
  shortcutMessage.value = '已恢复默认快捷键 Alt。'
}

function handleKeydown(event: KeyboardEvent) {
  if (isModifierOnlyShortcut(searchShortcut.value)) {
    if (normalizeShortcutKey(event.key) === searchShortcut.value && !event.repeat) modifierShortcutChorded = false
    else if (isConfiguredModifierPressed(event, searchShortcut.value)) modifierShortcutChorded = true
  } else if (
    !event.repeat
    && shortcutFromKeyboardEvent(event) === searchShortcut.value
    && canOpenSearchFromShortcut(event)
  ) {
    event.preventDefault()
    void openGallerySearchDialog()
    return
  }

  if (event.key === 'Escape' && mobileMoreOpen.value) {
    mobileMoreOpen.value = false
    return
  }
  if (!lightbox.value?.open) return
  if (event.key === 'ArrowLeft') stepPhoto(-1)
  if (event.key === 'ArrowRight') stepPhoto(1)
  if (event.key.toLowerCase() === 'f' && currentPhoto.value) void togglePhotoFavorite(currentPhoto.value)
  if (event.code === 'Space' && !event.repeat) {
    event.preventDefault()
    toggleSlideshow()
  }
}

function handleKeyup(event: KeyboardEvent) {
  if (!isModifierOnlyShortcut(searchShortcut.value) || normalizeShortcutKey(event.key) !== searchShortcut.value) return
  const shouldOpenSearch = !modifierShortcutChorded && canOpenSearchFromShortcut(event)
  modifierShortcutChorded = false
  if (shouldOpenSearch) {
    event.preventDefault()
    void openGallerySearchDialog()
  }
}

watch(wheelZoomModifier, value => localStorage.setItem('airy-wheel-zoom-modifier', value))

watch(theme, (value) => {
  document.documentElement.dataset.theme = value
  localStorage.setItem('airy-theme', value)
  applyMaterialTheme(themeSeed.value, value)
})

watch(activeView, async () => {
  mobileMoreOpen.value = false
  await nextTick()
  maybeLoadMore()
})

watch([slideshowPlaying, previewIndex], scheduleSlideshow)
watch([previewPhotos, previewIndex], preloadUpcomingPhotos, { flush: 'post' })
watch(personName, () => {
  window.clearTimeout(newPersonSearchTimer)
  newPersonSelectedAlbumIDs.value = new Set()
  if (activeView.value !== 'people' || selectedPerson.value || !personName.value.trim()) {
    newPersonAlbumOptions.value = []
    return
  }
  newPersonSearchTimer = window.setTimeout(() => void loadNewPersonAlbumOptions(), 250)
})
watch(seriesName, () => {
  window.clearTimeout(newSeriesSearchTimer)
  newSeriesSelectedAlbumIDs.value = new Set()
  if (activeView.value !== 'series' || selectedSeries.value || !seriesName.value.trim()) {
    newSeriesAlbumOptions.value = []
    return
  }
  newSeriesSearchTimer = window.setTimeout(() => void loadNewSeriesAlbumOptions(), 250)
})

onMounted(async () => {
  const savedWheelZoomModifier = localStorage.getItem('airy-wheel-zoom-modifier')
  if (isWheelZoomModifier(savedWheelZoomModifier)) wheelZoomModifier.value = savedWheelZoomModifier
  const savedThemeSeed = normalizeThemeSeed(localStorage.getItem(THEME_SEED_STORAGE_KEY)) ?? DEFAULT_THEME_SEED
  themeSeed.value = savedThemeSeed
  themeSeedDraft.value = savedThemeSeed

  const savedTheme = localStorage.getItem('airy-theme')
  theme.value =
    savedTheme === 'dark' || savedTheme === 'light'
      ? savedTheme
      : window.matchMedia('(prefers-color-scheme: dark)').matches
        ? 'dark'
        : 'light'
  applyMaterialTheme(themeSeed.value, theme.value)

  const savedSearchShortcut = localStorage.getItem(searchShortcutStorageKey)
  if (savedSearchShortcut && savedSearchShortcut.length <= 64) {
    searchShortcut.value = savedSearchShortcut
    searchShortcutDraft.value = savedSearchShortcut
  }

  localStorage.removeItem('airy-favorites')

  document.addEventListener('keydown', handleKeydown)
  document.addEventListener('keyup', handleKeyup)
  document.addEventListener('visibilitychange', handleVisibilityChange)
  window.addEventListener('popstate', handlePopState)
  window.addEventListener('airy:unauthorized', handleUnauthorized)

  try {
    const status = await fetchAuthStatus()
    if (!status.configured) {
      authState.value = 'setup'
      return
    }
    if (!status.authenticated) {
      authState.value = 'login'
      return
    }
    authState.value = 'authenticated'
    authUsername.value = status.username ?? ''
    accountUsername.value = authUsername.value
  } catch (error) {
    authState.value = 'login'
    authError.value = error instanceof Error ? error.message : '无法读取登录状态'
    return
  }

  if (window.location.pathname === '/trash' || window.location.pathname === '/trash/') {
    window.history.replaceState({}, '', '/')
  }

  let routeLoad: Promise<void> | undefined
  const personRoute = window.location.pathname.match(/^\/people\/(\d+)\/?$/)
  const seriesRoute = window.location.pathname.match(/^\/series\/(\d+)\/?$/)
  if (personRoute) {
    activeView.value = 'people'
    selectedPersonID.value = Number(personRoute[1])
    routeLoad = loadPeople()
  } else if (window.location.pathname === '/people' || window.location.pathname === '/people/') {
    activeView.value = 'people'
    routeLoad = loadPeople()
  } else if (seriesRoute) {
    activeView.value = 'series'
    selectedSeriesID.value = Number(seriesRoute[1])
    routeLoad = loadSeries()
  } else if (window.location.pathname === '/series' || window.location.pathname === '/series/') {
    activeView.value = 'series'
    routeLoad = loadSeries()
  } else {
    activeView.value = viewFromPath(window.location.pathname)
    if (activeView.value === 'photos') {
      const search = new URLSearchParams(window.location.search).get('q')?.trim() ?? ''
      gallerySearch.value = search
      gallerySearchInput.value = search
    }
    if (activeView.value === 'favorites') routeLoad = loadFavoriteGallery(true)
  }

  await Promise.allSettled([
    (async () => {
      try {
        const response = await fetch('/api/health')
        apiState.value = response.ok ? 'online' : 'offline'
      } catch {
        apiState.value = 'offline'
      }
    })(),
    loadGallery(true),
    loadDirectories(),
    loadScanSchedule(),
    routeLoad ?? Promise.resolve(),
  ])
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleKeydown)
  document.removeEventListener('keyup', handleKeyup)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  window.removeEventListener('popstate', handlePopState)
  window.removeEventListener('airy:unauthorized', handleUnauthorized)
  window.clearTimeout(directoryRefreshTimer)
  window.clearTimeout(slideshowTimer)
  window.clearTimeout(newPersonSearchTimer)
  window.clearTimeout(newSeriesSearchTimer)
  clearLightboxPreloads()
})
</script>

<template>
  <div v-if="authState === 'loading'" class="auth-pending" aria-busy="true"></div>

  <div v-else-if="authState !== 'authenticated'" class="auth-shell">
    <main class="auth-card">
      <img class="auth-logo" src="/airy-logo.png" alt="" />
      <h1>{{ authState === 'setup' ? '创建管理员' : '登录 Airy' }}</h1>
      <form class="auth-form" @submit.prevent="submitAuthentication">
        <label>
          <span>用户名</span>
          <input v-model="authUsername" type="text" autocomplete="username" maxlength="64" required autofocus />
        </label>
        <label>
          <span>密码</span>
          <input
            v-model="authPassword"
            type="password"
            :autocomplete="authState === 'setup' ? 'new-password' : 'current-password'"
            minlength="8"
            maxlength="72"
            required
          />
        </label>
        <label v-if="authState === 'setup'">
          <span>确认密码</span>
          <input v-model="authPasswordConfirm" type="password" autocomplete="new-password" minlength="8" maxlength="72" required />
        </label>
        <p v-if="authError" class="auth-error" role="alert">{{ authError }}</p>
        <button class="primary-action" type="submit" :disabled="authSaving || !authUsername.trim() || !authPassword">
          {{ authSaving ? '请稍候…' : authState === 'setup' ? '创建并进入图库' : '登录' }}
        </button>
      </form>
    </main>
  </div>

  <div v-else class="app-layout">
    <aside class="sidebar">
      <a class="brand" href="/" aria-label="Airy 首页">
        <span class="brand-mark" aria-hidden="true">
          <img src="/airy-logo.png" alt="" />
        </span>
        <span class="brand-name">Airy</span>
      </a>

      <nav aria-label="主导航">
        <button :class="['nav-item', { active: activeView === 'favorites' }]" type="button" @click="setView('favorites')">
          <MdIcon name="favorite" />
          <span>收藏</span>
        </button>
        <button :class="['nav-item', { active: activeView === 'photos' && !gallerySearch }]" type="button" @click="showPhotosHome">
          <MdIcon name="photo" />
          <span>最新</span>
        </button>
        <button :class="['nav-item', { active: activeView === 'people' }]" type="button" @click="setView('people')">
          <MdIcon name="face" />
          <span>模特</span>
        </button>
        <button :class="['nav-item', { active: activeView === 'series' }]" type="button" @click="setView('series')">
          <MdIcon name="layers" />
          <span>系列</span>
        </button>
        <button
          :class="['nav-item', { active: activeView === 'photos' && !!gallerySearch, 'has-state': gallerySearch }]"
          type="button"
          :title="`搜索相册（${searchShortcutLabel}）`"
          :aria-keyshortcuts="searchShortcut"
          @click="openGallerySearchDialog"
        >
          <MdIcon name="search" />
          <span>搜索</span>
        </button>
        <button :class="['nav-item', 'nav-resource', { active: activeView === 'folders' }]" type="button" @click="setView('folders')">
          <MdIcon name="hardDisk" />
          <span>资源</span>
        </button>
        <button :class="['nav-item', 'nav-settings', { active: activeView === 'settings' }]" type="button" @click="setView('settings')">
          <MdIcon name="settings" />
          <span>设置</span>
        </button>
        <button
          :class="['nav-item', 'mobile-more-toggle', { active: activeView === 'folders' || activeView === 'settings' || mobileMoreOpen }]"
          type="button"
          aria-haspopup="menu"
          aria-controls="mobile-more-menu"
          :aria-expanded="mobileMoreOpen"
          @click="mobileMoreOpen = !mobileMoreOpen"
        >
          <MdIcon name="more" />
          <span>更多</span>
        </button>
      </nav>

      <button v-if="mobileMoreOpen" class="mobile-more-scrim" type="button" aria-label="关闭更多菜单" @click="mobileMoreOpen = false"></button>
      <Transition name="mobile-more">
        <div v-if="mobileMoreOpen" id="mobile-more-menu" class="mobile-more-menu" role="menu">
          <button :class="{ active: activeView === 'folders' }" type="button" role="menuitem" @click="openMobileResource">
            <MdIcon name="hardDisk" />
            <span><b>资源</b><small>管理资源库与扫描</small></span>
          </button>
          <button :class="{ active: activeView === 'settings' }" type="button" role="menuitem" @click="openMobileSettings">
            <MdIcon name="settings" />
            <span><b>设置</b><small>账户、安全与退出登录</small></span>
          </button>
          <button type="button" role="menuitem" @click="toggleMobileTheme">
            <MdIcon v-if="theme === 'light'" name="darkMode" />
            <MdIcon v-else name="lightMode" />
            <span><b>{{ theme === 'light' ? '深色主题' : '浅色主题' }}</b><small>切换网站显示外观</small></span>
          </button>
        </div>
      </Transition>

      <div class="sidebar-footer">
        <button class="theme-toggle" type="button" :aria-label="theme === 'light' ? '切换到深色模式' : '切换到浅色模式'" @click="toggleTheme">
          <MdIcon v-if="theme === 'light'" name="lightMode" />
          <MdIcon v-else name="darkMode" />
        </button>
        <span class="connection" :data-state="apiState" :title="apiState === 'online' ? '服务已连接' : apiState === 'offline' ? '服务未连接' : '正在连接'">
          <i></i>
          <span>{{ apiState === 'online' ? '已连接' : apiState === 'offline' ? '离线' : '连接中' }}</span>
        </span>
      </div>
    </aside>

    <main ref="content" class="content" :class="{ 'content-empty': showOnlyEmptyState }" @scroll.passive="maybeLoadMore">
      <header v-if="!showOnlyEmptyState" class="page-header">
        <Transition name="header-copy" mode="out-in">
          <div :key="activeView">
            <p class="eyebrow">{{ currentMeta.eyebrow }}</p>
            <div class="page-title-row">
              <h1>{{ currentMeta.title }}</h1>
            </div>
            <p
              v-if="activeView === 'people' && selectedPerson && (personIdentityLabel(selectedPerson) || selectedPerson.aliases.length)"
              class="person-detail-aliases"
            >
              <span v-if="personIdentityLabel(selectedPerson)">{{ personIdentityLabel(selectedPerson) }}</span>
              <span v-if="personIdentityLabel(selectedPerson) && selectedPerson.aliases.length" aria-hidden="true"> · </span>
              <span v-if="selectedPerson.aliases.length">别名：{{ selectedPerson.aliases.join('、') }}</span>
            </p>
            <p v-if="activeView === 'people' && selectedPerson" class="summary person-detail-summary">
              <button
                class="summary-edit-button"
                type="button"
                title="修改模特信息"
                aria-label="修改模特信息"
                :disabled="personSaving"
                @click="openPersonEditDialog"
              >
                <MdIcon name="edit" />
              </button>
              <span aria-hidden="true">·</span>
              <span>{{ selectedPerson.album_count }} 个相簿</span>
              <span aria-hidden="true">·</span>
              <span>{{ personTotalMedia }} 个项目</span>
            </p>
            <p v-else-if="activeView === 'series' && selectedSeries" class="summary person-detail-summary">
              <button
                class="summary-edit-button"
                type="button"
                title="修改系列信息"
                aria-label="修改系列信息"
                :disabled="seriesSaving"
                @click="openSeriesEditDialog"
              >
                <MdIcon name="edit" />
              </button>
              <span aria-hidden="true">·</span>
              <span>{{ selectedSeries.album_count }} 个相簿</span>
              <span aria-hidden="true">·</span>
              <span>{{ seriesTotalMedia }} 个项目</span>
            </p>
            <p v-else class="summary">{{ summary }}</p>
          </div>
        </Transition>
      </header>

      <Transition name="view" mode="out-in">
        <div :key="activeView" class="view-stage">
      <section
        v-if="(activeView === 'photos' && galleryLoading) || (activeView === 'favorites' && favoriteGalleryLoading)"
        class="gallery-state"
        aria-live="polite"
      >
        <span class="loading-spinner" aria-hidden="true"></span>
        <h2>{{ activeView === 'favorites' ? '正在读取收藏相册' : gallerySearch ? '正在搜索相册' : '正在整理图库' }}</h2>
        <p>{{ activeView === 'favorites' ? '已收藏的相册会显示在这里。' : gallerySearch ? `正在查找标题包含“${gallerySearch}”的相册。` : '首次扫描时，相簿会随着扫描进度逐步出现。' }}</p>
      </section>

      <section
        v-else-if="activeView === 'photos' && galleryError && !groups.length"
        class="gallery-state error-state"
        role="alert"
      >
        <h2>暂时无法读取图库</h2>
        <p>{{ galleryError }}</p>
        <button class="primary-action" type="button" @click="loadGallery(true)">重新加载</button>
      </section>

      <section
        v-else-if="activeView === 'favorites' && favoriteGalleryError && !visibleGroups.length"
        class="gallery-state error-state"
        role="alert"
      >
        <h2>暂时无法读取收藏相册</h2>
        <p>{{ favoriteGalleryError }}</p>
        <button class="primary-action" type="button" @click="loadFavoriteGallery(true)">重新加载</button>
      </section>

      <section v-else-if="activeView === 'settings'" class="settings-view">
        <form class="settings-card" @submit.prevent="saveAccountSettings">
          <header>
            <span class="settings-icon" aria-hidden="true">
              <MdIcon name="person" />
            </span>
            <div>
              <h2>管理员账户</h2>
              <p>修改用户名或密码。保存后会退出其他设备上的登录会话。</p>
            </div>
          </header>
          <div class="settings-fields">
            <label>
              <span>用户名</span>
              <input v-model="accountUsername" type="text" autocomplete="username" maxlength="64" required />
            </label>
            <label>
              <span>当前密码</span>
              <input v-model="accountCurrentPassword" type="password" autocomplete="current-password" maxlength="72" required />
            </label>
            <label>
              <span>新密码</span>
              <input v-model="accountNewPassword" type="password" autocomplete="new-password" maxlength="72" placeholder="不修改请留空" />
            </label>
            <label>
              <span>确认新密码</span>
              <input v-model="accountNewPasswordConfirm" type="password" autocomplete="new-password" maxlength="72" :required="!!accountNewPassword" />
            </label>
          </div>
          <p v-if="accountError" class="auth-error" role="alert">{{ accountError }}</p>
          <p v-if="accountMessage" class="settings-success" role="status">{{ accountMessage }}</p>
          <div class="settings-actions">
            <button class="primary-action" type="submit" :disabled="accountSaving || !accountUsername.trim() || !accountCurrentPassword">
              {{ accountSaving ? '正在保存…' : '保存账户设置' }}
            </button>
            <button class="logout-button" type="button" :disabled="authSaving" @click="signOut">退出登录</button>
          </div>
        </form>

        <section class="settings-card">
          <header>
            <span class="settings-icon" aria-hidden="true">
              <MdIcon name="lightMode" />
            </span>
            <div>
              <h2>自定义主题</h2>
              <p>选择一个种子色，按照 Material Design 3 自动生成浅色与深色配色角色。</p>
            </div>
          </header>
          <div class="theme-setting">
            <label class="theme-color-control">
              <input
                class="theme-color-input"
                type="color"
                :value="themeSeedDraft"
                aria-label="选择主题种子色"
                @input="handleThemeColorInput"
              />
              <span>
                <b>主题种子色</b>
                <code>{{ themeSeedDraft.toUpperCase() }}</code>
              </span>
            </label>
            <div class="theme-role-preview" aria-label="主题颜色预览">
              <i class="theme-role-primary"></i>
              <i class="theme-role-container"></i>
              <i class="theme-role-surface"></i>
              <i class="theme-role-text"></i>
            </div>
          </div>
          <div class="theme-presets" aria-label="预设主题颜色">
            <button
              v-for="preset in THEME_PRESETS"
              :key="preset.value"
              :class="{ selected: themeSeedDraft === preset.value }"
              type="button"
              :aria-pressed="themeSeedDraft === preset.value"
              @click="selectThemeSeed(preset.value)"
            >
              <i :style="{ backgroundColor: preset.value }"></i>
              <span>{{ preset.name }}</span>
            </button>
          </div>
        </section>

        <section class="settings-card">
          <header>
            <span class="settings-icon" aria-hidden="true"><MdIcon name="zoomIn" /></span>
            <div>
              <h2>滚轮缩放辅助键</h2>
              <p>预览图片时，按住所选辅助键并滚动滚轮进行缩放。选择后自动保存到当前浏览器。</p>
            </div>
          </header>
          <div class="wheel-zoom-options" role="radiogroup" aria-label="滚轮缩放辅助键">
            <button v-for="(label, key) in wheelZoomLabels" :key="key" class="secondary-action" type="button"
              role="radio" :aria-checked="wheelZoomModifier === key" :class="{ selected: wheelZoomModifier === key }"
              @click="wheelZoomModifier = key">{{ label }}</button>
          </div>
        </section>

        <form class="settings-card" @submit.prevent="saveSearchShortcut">
          <header>
            <span class="settings-icon" aria-hidden="true">
              <MdIcon name="keyboard" />
            </span>
            <div>
              <h2>搜索快捷键</h2>
              <p>点击输入框后按下单个按键或组合键。默认使用 Alt。</p>
            </div>
          </header>
          <div class="shortcut-setting">
            <label>
              <span>呼出相册搜索</span>
              <input
                class="shortcut-input"
                type="text"
                :value="searchShortcutDraftLabel"
                readonly
                aria-label="录入搜索快捷键"
                @keydown="captureSearchShortcut"
                @focus="shortcutMessage = ''"
              />
            </label>
            <p>聚焦输入框并直接按键；组合键需要同时按下。</p>
          </div>
          <p v-if="shortcutMessage" class="shortcut-message" role="status">{{ shortcutMessage }}</p>
          <div class="settings-actions">
            <button class="primary-action" type="submit" :disabled="searchShortcutDraft === searchShortcut">保存快捷键</button>
            <button class="secondary-action" type="button" @click="resetSearchShortcut">恢复默认</button>
          </div>
        </form>
      </section>

      <section v-else-if="activeView === 'folders'" class="resource-manager">
        <form class="add-folder-card" @submit.prevent="addDirectory">
          <span class="folder-illustration" aria-hidden="true">
            <MdIcon name="createFolder" />
          </span>
          <div class="add-folder-copy">
            <h2>添加资源文件夹</h2>
            <p>填写服务器上的绝对路径，并选择作为相簿的子目录层级。</p>
          </div>
          <div class="folder-input-row">
            <input
              v-model="directoryPath"
              type="text"
              name="directory-path"
              placeholder="/mnt/photos/library"
              autocomplete="off"
              :disabled="directorySaving"
              aria-label="资源文件夹绝对路径"
            />
            <label class="album-depth-input">
              <span>相册深度</span>
              <select v-model.number="directoryAlbumDepth" :disabled="directorySaving" aria-label="相册目录深度">
                <option v-for="depth in albumDepthOptions" :key="depth" :value="depth">{{ depth }} 级</option>
              </select>
            </label>
            <button class="primary-action" type="submit" :disabled="!directoryPath.trim() || directorySaving">
              {{ directorySaving ? '正在添加…' : '添加文件夹' }}
            </button>
          </div>
          <p class="folder-safety">添加后不会自动扫描，也不会修改或移动原始文件。</p>
        </form>

        <p v-if="directoryError" class="inline-error folder-error" role="alert">{{ directoryError }}</p>

        <div class="folder-manager-toolbar">
          <div class="global-scan-copy">
            <b>全局相册 CSV</b>
            <span>导出全部资源库的相册，编辑后按 ID 批量导入。</span>
          </div>
          <AlbumCsvActions @applied="loadDirectories(true)" />
        </div>

        <div v-if="directories.length" class="folder-manager-toolbar">
          <div class="global-scan-copy">
            <b>全局图库扫描</b>
            <span>每天夜间 01:00，只扫描未关闭监听和扫描的资源文件夹。</span>
            <small v-if="scanSchedule.enabled && scanSchedule.next_scan_at">
              下次扫描：{{ formatScheduledTime(scanSchedule.next_scan_at) }}（北京时间）
            </small>
          </div>
          <div class="global-scan-settings">
            <label class="watch-setting">
              <input
                type="checkbox"
                :checked="scanSchedule.enabled"
                :disabled="scanScheduleSaving"
                @change="changeScanScheduleEnabled"
              />
              <span class="switch-track" aria-hidden="true"><i></i></span>
              <span>定时扫描</span>
            </label>
            <span class="daily-schedule-time">每天 {{ scanSchedule.scan_time }} · 北京时间</span>
          </div>
          <div class="global-scan-actions">
            <template v-if="directoryScanRunning">
              <button
                class="cancel-scan-button"
                type="button"
                :disabled="scanControlChanging || activeScan?.canceling"
                @click="cancelActiveScan"
              >
                {{ activeScan?.canceling ? '正在取消…' : '取消扫描' }}
              </button>
            </template>
            <button
              v-else
              class="primary-action"
              type="button"
              :disabled="globalScanStarting"
              @click="startGlobalScan"
            >
              {{ globalScanStarting ? '正在加入队列…' : '扫描未关闭的文件夹' }}
            </button>
          </div>
        </div>

        <div v-if="directoriesLoading && !directories.length" class="folder-loading" aria-live="polite">
          <span class="loading-spinner small" aria-hidden="true"></span>
          <span>正在读取资源文件夹…</span>
        </div>

        <div v-else-if="directories.length" class="folder-list">
          <article v-for="directory in directories" :key="directory.id" class="folder-card">
            <span class="folder-card-icon" aria-hidden="true">
              <MdIcon name="folder" />
            </span>
            <div class="folder-card-main">
              <div class="folder-title-row">
                <h2>{{ directory.name }}</h2>
                <span :class="['folder-status', { scanning: directory.scanning || directory.queued, ready: directory.scan_status && !directory.scanning && !directory.queued, failed: directory.scan_error }]">
                  {{ directory.scanning ? scanStageLabel(activeScan?.stage) : directory.queued ? '等待扫描' : directory.scan_error ? '扫描失败' : directory.scan_status ? '已就绪' : '待扫描' }}
                </span>
                <span v-if="directory.automation_disabled" class="folder-status excluded">已排除</span>
                <span v-else :class="['folder-status', 'watching', { failed: directory.watch_error }]">
                  {{ directory.watch_error ? '监听异常' : directory.watching ? '监听中' : '正在启用监听' }}
                </span>
              </div>
              <p class="folder-path" :title="directory.path">{{ directory.path }}</p>
              <div class="folder-meta">
                <span><b>{{ directory.album_count }}</b> 个相簿</span>
                <span><b>{{ directory.media_count }}</b> 个项目</span>
                <span>第 <b>{{ directory.album_depth }}</b> 级目录作为相册</span>
                <span>{{ formatScanTime(directory.last_scanned_at) }}</span>
              </div>
              <div class="folder-automation">
                <label class="watch-setting">
                  <input
                    type="checkbox"
                    :checked="directory.automation_disabled"
                    :disabled="directorySettingsSaving.has(directory.id) || directory.scanning || directory.queued"
                    @change="changeDirectoryAutomation(directory, $event)"
                  />
                  <span class="switch-track" aria-hidden="true"><i></i></span>
                  <span>关闭监听和扫描</span>
                </label>
                <span v-if="directorySettingsSaving.has(directory.id)" class="settings-saving">保存中…</span>
              </div>
              <p v-if="directory.watch_error" class="folder-scan-error">监听错误：{{ directory.watch_error }}</p>
              <p v-if="directory.scan_error" class="folder-scan-error">{{ directory.scan_error }}</p>
              <p v-if="directory.scanning" class="scan-stage-copy">{{ scanStageDetail(activeScan) }}</p>
              <div v-if="directory.scanning" class="scan-progress" aria-live="polite">
                <span></span>
              </div>
            </div>
            <div class="folder-actions">
              <button
                class="scan-button"
                type="button"
                :disabled="directoryScanRunning"
                @click="startDirectoryScan(directory)"
              >
                <MdIcon name="refresh" />
                {{ directory.scanning ? '扫描中' : '扫描' }}
              </button>
              <button
                class="remove-folder-button"
                type="button"
                :disabled="directory.scanning"
                aria-label="移除资源文件夹"
                @click="removeDirectory(directory)"
              >
                <MdIcon name="delete" />
              </button>
            </div>
          </article>
        </div>

        <div v-else class="folder-empty">
          <h2>还没有资源文件夹</h2>
          <p>在上方输入服务器目录路径，添加后再手动开始第一次扫描。</p>
        </div>
      </section>

      <section v-else-if="activeView === 'people'" class="people-view">
        <template v-if="!selectedPerson">
          <div v-if="!showOnlyEmptyState" class="people-manager people-list-toolbar">
            <button class="entity-favorite-filter" :class="{ active: peopleFavoritesOnly }"
              type="button" :aria-pressed="peopleFavoritesOnly" @click="peopleFavoritesOnly = !peopleFavoritesOnly">
              <MdIcon :name="peopleFavoritesOnly ? 'favorite' : 'favoriteBorder'" />
              <span>仅收藏</span>
            </button>
            <div class="people-toolbar-actions">
              <label class="people-search">
                <MdIcon name="search" />
                <input
                  v-model="peopleSearch"
                  type="search"
                  placeholder="搜索模特、备注或别名，空格分隔关键词"
                  autocomplete="off"
                  aria-label="搜索模特"
                />
                <button v-if="peopleSearch" type="button" aria-label="清除搜索" @click="peopleSearch = ''">
                  <MdIcon name="close" />
                </button>
              </label>
              <button
                class="primary-action add-person-button"
                type="button"
                aria-label="添加模特"
                @click="openPersonCreateDialog"
              >
                <MdIcon name="personAdd" />
                <span class="add-person-button-label">添加模特</span>
              </button>
            </div>
          </div>

          <dialog
            ref="personCreateDialog"
            class="person-create-dialog"
            @click="closePersonCreateFromBackdrop"
            @close="resetPersonCreateForm"
          >
            <div class="person-create-modal">
              <header class="person-create-modal-header">
                <div>
                  <h2>添加模特</h2>
                  <p>主名称可重复；备注用于区分同名模特，别名用于搜索</p>
                </div>
                <button type="button" aria-label="关闭添加模特" :disabled="personSaving" @click="closePersonCreateDialog">
                  <MdIcon name="close" />
                </button>
              </header>
            <form class="person-create person-create-expanded" @submit.prevent="addPerson">
              <div class="person-create-row">
                <input
                  v-model="personName"
                  type="text"
                  maxlength="255"
                  placeholder="输入模特名称，自动匹配相册标题"
                  autocomplete="off"
                  :disabled="personSaving"
                  aria-label="模特名称"
                />
                <span v-if="newPersonAlbumSearchLoading" class="loading-spinner small" aria-hidden="true"></span>
                <button class="primary-action" type="submit" :disabled="!personName.trim() || personSaving">
                  {{ personSaving ? '保存中…' : newPersonSelectedAlbumIDs.size ? `添加 · ${newPersonSelectedAlbumIDs.size} 个相册` : '添加模特' }}
                </button>
              </div>
              <div class="person-identity-fields">
                <label>
                  <span>区分备注</span>
                  <input
                    v-model="personNote"
                    type="text"
                    maxlength="255"
                    placeholder="例如：资源库 A、1998 年生"
                    autocomplete="off"
                    :disabled="personSaving"
                  />
                </label>
                <label>
                  <span>别名</span>
                  <input
                    v-model="personAliases"
                    type="text"
                    placeholder="多个别名使用逗号分隔"
                    autocomplete="off"
                    :disabled="personSaving"
                  />
                </label>
              </div>
              <div
                v-if="personName.trim() && !newPersonAlbumSearchLoading"
                class="person-album-selection-toolbar"
              >
                <p>
                  找到 {{ newPersonAlbumOptions.length }} 个标题包含“{{ personName.trim() }}”的相册，可多选后一起关联。
                </p>
                <button
                  v-if="newPersonAlbumOptions.length"
                  class="select-all-albums-button"
                  type="button"
                  :aria-pressed="allNewPersonAlbumsSelected"
                  :disabled="personSaving"
                  @click="toggleAllNewPersonAlbums"
                >
                  <span
                    :class="['album-option-check', {
                      selected: allNewPersonAlbumsSelected,
                      partial: someNewPersonAlbumsSelected,
                    }]"
                    aria-hidden="true"
                  >
                    <MdIcon v-if="allNewPersonAlbumsSelected" name="check" />
                    <MdIcon v-else-if="someNewPersonAlbumsSelected" name="remove" />
                  </span>
                  <span>{{ allNewPersonAlbumsSelected ? '取消全选' : '全选' }}</span>
                  <small>{{ selectedNewPersonAlbumCount }}/{{ newPersonAlbumOptions.length }}</small>
                </button>
              </div>
              <div v-if="newPersonAlbumOptions.length" class="new-person-album-list">
                <button
                  v-for="album in newPersonAlbumOptions"
                  :key="album.id"
                  :class="['new-person-album-option', { selected: newPersonSelectedAlbumIDs.has(album.id) }]"
                  type="button"
                  :aria-pressed="newPersonSelectedAlbumIDs.has(album.id)"
                  @click="toggleNewPersonAlbum(album.id)"
                >
                  <span class="album-option-check">
                    <MdIcon v-if="newPersonSelectedAlbumIDs.has(album.id)" name="check" />
                  </span>
                  <span>{{ album.name }}</span>
                </button>
              </div>
              <p v-if="personError" class="person-create-error" role="alert">{{ personError }}</p>
            </form>
            </div>
          </dialog>

          <p v-if="personError" class="inline-error people-error" role="status">{{ personError }}</p>

          <div v-if="peopleLoading && !people.length" class="gallery-state" aria-live="polite">
            <span class="loading-spinner" aria-hidden="true"></span>
            <h2>正在读取模特</h2>
          </div>

          <section v-else-if="!people.length" class="empty-state">
            <span class="empty-icon">
              <MdIcon name="face" />
            </span>
            <h2>还没有模特</h2>
            <p>先创建模特，然后进入模特页面关联相册。</p>
            <button class="primary-action" type="button" @click="openPersonCreateDialog">添加模特</button>
          </section>

          <section v-else-if="!visiblePeople.length" class="empty-state people-search-empty">
            <span class="empty-icon">
              <MdIcon name="search" />
            </span>
            <h2>没有匹配的模特</h2>
            <p>请尝试调整关键词，或关闭“仅收藏”筛选。</p>
            <button class="primary-action" type="button" @click="peopleSearch = ''; peopleFavoritesOnly = false">清除筛选</button>
          </section>

          <div v-else class="people-grid">
            <article
              v-for="person in visiblePeople"
              :key="person.id"
              :class="['person-card', { favorite: person.favorite }]"
            >
              <button class="person-card-open" type="button" @click="openPerson(person.id)">
                <span class="person-avatar" aria-hidden="true">
                  <img v-if="person.avatar_url" :src="person.avatar_url" alt="" loading="lazy" fetchpriority="low" />
                  <span v-else>{{ person.name.slice(0, 1).toUpperCase() }}</span>
                </span>
                <span class="person-card-copy">
                  <b>{{ person.name }}</b>
                  <small v-if="personIdentityLabel(person)" class="person-identity-note">{{ personIdentityLabel(person) }}</small>
                  <small v-if="person.aliases.length" class="person-aliases">{{ person.aliases.join(' · ') }}</small>
                  <small>{{ person.album_count }} 个相册</small>
                </span>
              </button>
              <button
                :class="['person-favorite-button', { saved: person.favorite }]"
                type="button"
                :aria-label="person.favorite ? `取消收藏模特 ${person.name}` : `收藏模特 ${person.name}`"
                :aria-pressed="person.favorite"
                :disabled="favoriteSavingPersonIDs.has(person.id)"
                @click="togglePersonFavorite(person)"
              >
                <FavoriteIcon :key="person.id" :active="person.favorite" />
              </button>
            </article>
          </div>
        </template>

        <template v-else-if="selectedPerson">
          <dialog
            ref="personEditDialog"
            class="person-create-dialog"
            @cancel.prevent="closePersonEditDialog"
            @click="closePersonEditFromBackdrop"
            @close="resetPersonEditForm"
          >
            <div class="person-create-modal">
              <header class="person-create-modal-header">
                <div>
                  <h2>修改模特信息</h2>
                  <p>主名称可重复；备注用于区分同名模特，别名用于搜索</p>
                </div>
                <button type="button" aria-label="关闭修改模特信息" :disabled="personSaving" @click="closePersonEditDialog">
                  <MdIcon name="close" />
                </button>
              </header>
              <form class="person-create person-create-expanded" @submit.prevent="saveSelectedPerson">
                <div class="person-create-row">
                  <input
                    v-model="personEditName"
                    type="text"
                    maxlength="255"
                    placeholder="输入模特名称"
                    autocomplete="off"
                    autofocus
                    :disabled="personSaving"
                    aria-label="模特名称"
                  />
                  <button class="primary-action" type="submit" :disabled="!personEditName.trim() || personSaving">
                    {{ personSaving ? '保存中…' : '保存修改' }}
                  </button>
                </div>
                <div class="person-identity-fields">
                  <label>
                    <span>区分备注</span>
                    <input
                      v-model="personEditNote"
                      type="text"
                      maxlength="255"
                      placeholder="例如：资源库 A、1998 年生"
                      autocomplete="off"
                      :disabled="personSaving"
                    />
                  </label>
                  <label>
                    <span>别名</span>
                    <input
                      v-model="personEditAliases"
                      type="text"
                      placeholder="多个别名使用逗号分隔"
                      autocomplete="off"
                      :disabled="personSaving"
                    />
                  </label>
                </div>
                <label class="series-album-search-label" for="edit-person-album-search">搜索相册</label>
                <div class="people-search series-album-search">
                  <MdIcon name="search" aria-hidden="true" />
                  <input id="edit-person-album-search" v-model="editPersonAlbumSearch" type="search" placeholder="输入相册标题，留空搜索全部" autocomplete="off" :disabled="personSaving" @keydown.enter.prevent />
                  <button class="album-search-submit" type="button" :disabled="personSaving || editPersonAlbumLoading" @click="searchEditPersonAlbums">{{ editPersonAlbumLoading ? '搜索中…' : '搜索' }}</button>
                </div>
                <div v-if="editPersonLastSearch !== null && !editPersonAlbumLoading && !editPersonAlbumError" class="person-album-selection-toolbar">
                  <p>“{{ editPersonLastSearch || '全部相册' }}”的搜索结果：{{ editPersonAlbumOptions.length }} 个相册。勾选加入，取消勾选移除关联。</p>
                  <button v-if="selectableEditPersonAlbums.length" class="select-all-albums-button" type="button" :aria-pressed="allEditPersonAlbumsSelected" :disabled="personSaving" @click="toggleAllEditPersonAlbums">
                    <span :class="['album-option-check', { selected: allEditPersonAlbumsSelected, partial: someEditPersonAlbumsSelected }]" aria-hidden="true">
                      <MdIcon v-if="allEditPersonAlbumsSelected" name="check" />
                      <MdIcon v-else-if="someEditPersonAlbumsSelected" name="remove" />
                    </span>
                    <span>{{ allEditPersonAlbumsSelected ? '取消全选' : '全选' }}</span>
                    <small>{{ selectedEditPersonAlbumCount }}/{{ selectableEditPersonAlbums.length }}</small>
                  </button>
                </div>
                <p v-if="editPersonSelectedAlbumIDs.size || editPersonRemovedAlbumIDs.size" class="series-album-search-help">待加入 {{ editPersonSelectedAlbumIDs.size }} 个，待取消 {{ editPersonRemovedAlbumIDs.size }} 个，点击“保存修改”生效。</p>
                <p v-if="editPersonAlbumLoading" class="person-create-error" role="status">正在匹配相册…</p>
                <p v-if="editPersonAlbumError" class="person-create-error" role="alert">{{ editPersonAlbumError }}</p>
                <div v-if="editPersonAlbumOptions.length" class="new-person-album-list">
                  <button v-for="album in editPersonAlbumOptions" :key="album.id" :class="['new-person-album-option', { selected: editPersonAlbumSelected(album) }]" type="button" :disabled="personSaving" :aria-pressed="editPersonAlbumSelected(album)" @click="toggleEditPersonAlbum(album)">
                    <span class="album-option-check" aria-hidden="true"><MdIcon v-if="editPersonAlbumSelected(album)" name="check" /></span>
                    <span :title="album.name">{{ album.name }}{{ album.associated ? editPersonAlbumSelected(album) ? '（已加入）' : '（待取消）' : '' }}</span>
                  </button>
                </div>
                <p v-if="personError" class="person-create-error" role="alert">{{ personError }}</p>
                <div class="entity-edit-danger-zone">
                  <div>
                    <b>删除模特</b>
                    <span>只会删除模特及其相册关联，不会删除相册或媒体文件。</span>
                  </div>
                  <button type="button" :disabled="personSaving" @click="removeSelectedPerson">
                    <MdIcon name="delete" />
                    <span>删除模特</span>
                  </button>
                </div>
              </form>
            </div>
          </dialog>
          <p v-if="personError" class="inline-error people-error" role="status">{{ personError }}</p>

          <div v-if="personGalleryLoading && !personGroups.length" class="gallery-state" aria-live="polite">
            <span class="loading-spinner" aria-hidden="true"></span>
            <h2>正在读取模特相册</h2>
          </div>

          <div v-else-if="personGroups.length" class="photo-feed person-photo-feed">
            <VirtualPhotoFeed
              :groups="personGroups"
              :scroll-element="content"
              :editable-person-id="selectedPersonID"
              :person-album-saving-keys="personAlbumSavingKeys"
              :series-album-saving-keys="seriesAlbumSavingKeys"
              @add-person-album="openAlbumPersonPicker"
              @open-photo="openPhoto"
              @open-person="openPerson"
              @open-series="openSeries"
              @remove-person-album="detachAlbumPerson"
              @remove-series-album="detachAlbumSeries"
              @toggle-album-favorite="toggleAlbumFavorite"
              @toggle-photo-favorite="togglePhotoFavorite"
            />
            <div v-if="personHasMoreAlbums || personGalleryLoadingMore" class="infinite-loader" aria-live="polite">
              <span v-if="personGalleryLoadingMore" class="loading-spinner small" aria-hidden="true"></span>
              <span>{{ personGalleryLoadingMore ? '正在载入下一批完整相册…' : '继续下滑加载更多相册' }}</span>
            </div>
          </div>

          <section v-else class="empty-state person-empty-albums">
            <span class="empty-icon">
              <MdIcon name="folder" />
            </span>
            <h2>还没有关联相册</h2>
            <p>在上方搜索相册并勾选，关联后会使用与首页相同的布局展示。</p>
          </section>
        </template>
      </section>

      <section v-else-if="activeView === 'series'" class="people-view series-view">
        <template v-if="!selectedSeries">
          <div v-if="!showOnlyEmptyState" class="people-manager people-list-toolbar">
            <button class="entity-favorite-filter" :class="{ active: seriesFavoritesOnly }"
              type="button" :aria-pressed="seriesFavoritesOnly" @click="seriesFavoritesOnly = !seriesFavoritesOnly">
              <MdIcon :name="seriesFavoritesOnly ? 'favorite' : 'favoriteBorder'" />
              <span>仅收藏</span>
            </button>
            <div class="people-toolbar-actions">
              <label class="people-search">
                <MdIcon name="search" />
                <input
                  v-model="seriesSearch"
                  type="search"
                  placeholder="搜索系列，空格分隔关键词"
                  autocomplete="off"
                  aria-label="搜索系列"
                />
                <button v-if="seriesSearch" type="button" aria-label="清除搜索" @click="seriesSearch = ''">
                  <MdIcon name="close" />
                </button>
              </label>
              <button
                class="primary-action add-person-button"
                type="button"
                aria-label="添加系列"
                @click="openSeriesCreateDialog"
              >
                <MdIcon name="layers" />
                <span class="add-person-button-label">添加系列</span>
              </button>
            </div>
          </div>

          <dialog
            ref="seriesCreateDialog"
            class="person-create-dialog"
            @click="closeSeriesCreateFromBackdrop"
            @close="resetSeriesCreateForm"
          >
            <div class="person-create-modal">
              <header class="person-create-modal-header">
                <div>
                  <h2>添加系列</h2>
                  <p>输入名称后选择需要关联的相册</p>
                </div>
                <button type="button" aria-label="关闭添加系列" :disabled="seriesSaving" @click="closeSeriesCreateDialog">
                  <MdIcon name="close" />
                </button>
              </header>
              <form class="person-create person-create-expanded" @submit.prevent="addSeries">
                <div class="person-create-row">
                  <input
                    v-model="seriesName"
                    type="text"
                    maxlength="255"
                    placeholder="输入系列名称，自动匹配相册标题"
                    autocomplete="off"
                    :disabled="seriesSaving"
                    aria-label="系列名称"
                  />
                  <span v-if="newSeriesAlbumSearchLoading" class="loading-spinner small" aria-hidden="true"></span>
                  <button class="primary-action" type="submit" :disabled="!seriesName.trim() || seriesSaving">
                    {{ seriesSaving ? '保存中…' : newSeriesSelectedAlbumIDs.size ? `添加 · ${newSeriesSelectedAlbumIDs.size} 个相册` : '添加系列' }}
                  </button>
                </div>
                <div v-if="seriesName.trim() && !newSeriesAlbumSearchLoading" class="person-album-selection-toolbar">
                  <p>找到 {{ newSeriesAlbumOptions.length }} 个标题包含“{{ seriesName.trim() }}”的相册，可多选后一起关联。</p>
                  <button
                    v-if="newSeriesAlbumOptions.length"
                    class="select-all-albums-button"
                    type="button"
                    :aria-pressed="allNewSeriesAlbumsSelected"
                    :disabled="seriesSaving"
                    @click="toggleAllNewSeriesAlbums"
                  >
                    <span :class="['album-option-check', { selected: allNewSeriesAlbumsSelected, partial: someNewSeriesAlbumsSelected }]" aria-hidden="true">
                      <MdIcon v-if="allNewSeriesAlbumsSelected" name="check" />
                      <MdIcon v-else-if="someNewSeriesAlbumsSelected" name="remove" />
                    </span>
                    <span>{{ allNewSeriesAlbumsSelected ? '取消全选' : '全选' }}</span>
                    <small>{{ selectedNewSeriesAlbumCount }}/{{ newSeriesAlbumOptions.length }}</small>
                  </button>
                </div>
                <div v-if="newSeriesAlbumOptions.length" class="new-person-album-list">
                  <button
                    v-for="album in newSeriesAlbumOptions"
                    :key="album.id"
                    :class="['new-person-album-option', { selected: newSeriesSelectedAlbumIDs.has(album.id) }]"
                    type="button"
                    :aria-pressed="newSeriesSelectedAlbumIDs.has(album.id)"
                    @click="toggleNewSeriesAlbum(album.id)"
                  >
                    <span class="album-option-check">
                      <MdIcon v-if="newSeriesSelectedAlbumIDs.has(album.id)" name="check" />
                    </span>
                    <span>{{ album.name }}</span>
                  </button>
                </div>
                <p v-if="seriesError" class="person-create-error" role="alert">{{ seriesError }}</p>
              </form>
            </div>
          </dialog>

          <p v-if="seriesError" class="inline-error people-error" role="status">{{ seriesError }}</p>
          <div v-if="seriesLoading && !series.length" class="gallery-state" aria-live="polite">
            <span class="loading-spinner" aria-hidden="true"></span>
            <h2>正在读取系列</h2>
          </div>
          <section v-else-if="!series.length" class="empty-state">
            <span class="empty-icon"><MdIcon name="layers" /></span>
            <h2>还没有系列</h2>
            <p>创建系列并选择相册，之后可以在系列页面集中浏览。</p>
            <button class="primary-action" type="button" @click="openSeriesCreateDialog">添加系列</button>
          </section>
          <section v-else-if="!visibleSeries.length" class="empty-state people-search-empty">
            <span class="empty-icon"><MdIcon name="search" /></span>
            <h2>没有匹配的系列</h2>
            <p>请尝试调整关键词，或关闭“仅收藏”筛选。</p>
            <button class="primary-action" type="button" @click="seriesSearch = ''; seriesFavoritesOnly = false">清除筛选</button>
          </section>
          <div v-else class="people-grid series-grid">
            <article v-for="item in visibleSeries" :key="item.id" :class="['person-card', 'series-card', { favorite: item.favorite }]">
              <button class="person-card-open" type="button" @click="openSeries(item.id)">
                <span class="person-avatar series-cover" aria-hidden="true">
                  <img v-if="item.cover_url" :src="item.cover_url" alt="" loading="lazy" fetchpriority="low" />
                  <span v-else>{{ item.name.slice(0, 1).toUpperCase() }}</span>
                </span>
                <span class="person-card-copy"><b>{{ item.name }}</b><small>{{ item.album_count }} 个相册</small></span>
              </button>
              <button
                :class="['person-favorite-button', { saved: item.favorite }]"
                type="button"
                :aria-label="item.favorite ? `取消收藏系列 ${item.name}` : `收藏系列 ${item.name}`"
                :aria-pressed="item.favorite"
                :disabled="favoriteSavingSeriesIDs.has(item.id)"
                @click="toggleSeriesFavorite(item)"
              >
                <FavoriteIcon :key="item.id" :active="item.favorite" />
              </button>
            </article>
          </div>
        </template>

        <template v-else>
          <dialog
            ref="seriesEditDialog"
            class="person-create-dialog"
            @cancel.prevent="closeSeriesEditDialog"
            @click="closeSeriesEditFromBackdrop"
            @close="resetSeriesEditForm"
          >
            <div class="person-create-modal">
              <header class="person-create-modal-header">
                <div>
                  <h2>修改系列信息</h2>
                  <p>修改名称或选择相册，保存后统一更新</p>
                </div>
                <button type="button" aria-label="关闭修改系列信息" :disabled="seriesSaving" @click="closeSeriesEditDialog">
                  <MdIcon name="close" />
                </button>
              </header>
              <form class="person-create person-create-expanded" @submit.prevent="saveSelectedSeries">
                <div class="person-create-row">
                  <input
                    v-model="seriesEditName"
                    type="text"
                    maxlength="255"
                    placeholder="输入系列名称"
                    autocomplete="off"
                    autofocus
                    :disabled="seriesSaving"
                    aria-label="系列名称"
                  />
                  <button class="primary-action" type="submit" :disabled="!seriesEditName.trim() || seriesSaving">
                    {{ seriesSaving ? '保存中…' : '保存修改' }}
                  </button>
                </div>
                <div class="series-album-search-heading">
                  <label class="series-album-search-label" for="edit-series-album-search">搜索相册</label>
                  <label class="series-album-regex-toggle"><input v-model="editSeriesAlbumRegex" type="checkbox" :disabled="seriesSaving" />正则</label>
                </div>
                <div class="people-search series-album-search">
                  <MdIcon name="search" aria-hidden="true" />
                  <input
                    id="edit-series-album-search"
                    v-model="editSeriesAlbumSearch"
                    type="search"
                    :placeholder="editSeriesAlbumRegex ? '输入正则表达式，如 海边|泳池' : '输入相册标题关键词'"
                    aria-describedby="edit-series-search-help"
                    autocomplete="off"
                    :disabled="seriesSaving"
                    @keydown.enter.prevent
                  />
                  <button class="album-search-submit" type="button" :disabled="seriesSaving || editSeriesAlbumLoading" @click="searchEditSeriesAlbums">{{ editSeriesAlbumLoading ? '搜索中…' : '搜索' }}</button>
                </div>
                <p id="edit-series-search-help" class="series-album-search-help">{{ editSeriesAlbumRegex ? '直接输入正则表达式，如 ^MichaelWu.*(海边|泳池)$，默认忽略大小写。' : '输入后点击“搜索”，留空搜索全部；勾选“正则”可使用表达式。' }}</p>
                <div v-if="editSeriesLastSearch !== null && !editSeriesAlbumLoading && !editSeriesAlbumError" class="person-album-selection-toolbar">
                  <p>“{{ editSeriesLastSearch || '全部相册' }}”的搜索结果：{{ editSeriesAlbumOptions.length }} 个相册。勾选加入，取消勾选移除关联。</p>
                  <button v-if="selectableEditSeriesAlbums.length" class="select-all-albums-button" type="button" :aria-pressed="allEditSeriesAlbumsSelected" :disabled="seriesSaving" @click="toggleAllEditSeriesAlbums">
                    <span :class="['album-option-check', { selected: allEditSeriesAlbumsSelected, partial: someEditSeriesAlbumsSelected }]" aria-hidden="true">
                      <MdIcon v-if="allEditSeriesAlbumsSelected" name="check" />
                      <MdIcon v-else-if="someEditSeriesAlbumsSelected" name="remove" />
                    </span>
                    <span>{{ allEditSeriesAlbumsSelected ? '取消全选' : '全选' }}</span>
                    <small>{{ selectedEditSeriesAlbumCount }}/{{ selectableEditSeriesAlbums.length }}</small>
                  </button>
                </div>
                <p v-if="editSeriesSelectedAlbumIDs.size || editSeriesRemovedAlbumIDs.size" class="series-album-search-help">待加入 {{ editSeriesSelectedAlbumIDs.size }} 个，待取消 {{ editSeriesRemovedAlbumIDs.size }} 个，点击“保存修改”生效。</p>
                <p v-if="editSeriesAlbumLoading" class="person-create-error" role="status">正在匹配相册…</p>
                <p v-if="editSeriesAlbumError" class="person-create-error" role="alert">{{ editSeriesAlbumError }}</p>
                <div v-if="editSeriesAlbumOptions.length" class="new-person-album-list">
                  <button v-for="album in editSeriesAlbumOptions" :key="album.id" :class="['new-person-album-option', { selected: editSeriesAlbumSelected(album) }]" type="button" :disabled="seriesSaving" :aria-pressed="editSeriesAlbumSelected(album)" @click="toggleEditSeriesAlbum(album)">
                    <span class="album-option-check" aria-hidden="true"><MdIcon v-if="editSeriesAlbumSelected(album)" name="check" /></span>
                    <span :title="album.name">{{ album.name }}{{ album.associated ? editSeriesAlbumSelected(album) ? '（已加入）' : '（待取消）' : '' }}</span>
                  </button>
                </div>
                <p v-if="seriesError" class="person-create-error" role="alert">{{ seriesError }}</p>
                <div class="entity-edit-danger-zone">
                  <div>
                    <b>删除系列</b>
                    <span>只会删除系列及其关联，不会删除相册或媒体文件。</span>
                  </div>
                  <button type="button" :disabled="seriesSaving" @click="removeSelectedSeries">
                    <MdIcon name="delete" />
                    <span>删除系列</span>
                  </button>
                </div>
              </form>
            </div>
          </dialog>
          <p v-if="seriesError" class="inline-error people-error" role="status">{{ seriesError }}</p>
          <div v-if="seriesGalleryLoading && !seriesGroups.length" class="gallery-state" aria-live="polite">
            <span class="loading-spinner" aria-hidden="true"></span>
            <h2>正在读取系列相册</h2>
          </div>
          <div v-else-if="seriesGroups.length" class="photo-feed person-photo-feed">
            <VirtualPhotoFeed
              :groups="seriesGroups"
              :scroll-element="content"
              :editable-series-id="selectedSeriesID"
              :person-album-saving-keys="personAlbumSavingKeys"
              :series-album-saving-keys="seriesAlbumSavingKeys"
              @add-person-album="openAlbumPersonPicker"
              @open-photo="openPhoto"
              @open-person="openPerson"
              @open-series="openSeries"
              @remove-person-album="detachAlbumPerson"
              @remove-series-album="detachAlbumSeries"
              @toggle-album-favorite="toggleAlbumFavorite"
              @toggle-photo-favorite="togglePhotoFavorite"
            />
            <div v-if="seriesHasMoreAlbums || seriesGalleryLoadingMore" class="infinite-loader" aria-live="polite">
              <span v-if="seriesGalleryLoadingMore" class="loading-spinner small" aria-hidden="true"></span>
              <span>{{ seriesGalleryLoadingMore ? '正在载入下一批完整相册…' : '继续下滑加载更多相册' }}</span>
            </div>
          </div>
          <section v-else class="empty-state person-empty-albums">
            <span class="empty-icon"><MdIcon name="folder" /></span>
            <h2>还没有关联相册</h2>
            <p>可以删除后重新创建系列并选择相册。</p>
          </section>
        </template>
      </section>

      <template v-else-if="activeView === 'photos' || activeView === 'favorites'">
        <p
          v-if="activeView === 'photos' ? galleryError : favoriteGalleryError"
          class="inline-error"
          role="status"
        >
          {{ activeView === 'photos' ? galleryError : favoriteGalleryError }}
        </p>
        <div v-if="visibleGroups.length" class="photo-feed">
          <VirtualPhotoFeed
            :groups="visibleGroups"
            :scroll-element="content"
            :person-album-saving-keys="personAlbumSavingKeys"
            :series-album-saving-keys="seriesAlbumSavingKeys"
            @add-person-album="openAlbumPersonPicker"
            @open-photo="openPhoto"
            @open-person="openPerson"
            @open-series="openSeries"
            @remove-person-album="detachAlbumPerson"
            @remove-series-album="detachAlbumSeries"
            @toggle-album-favorite="toggleAlbumFavorite"
            @toggle-photo-favorite="togglePhotoFavorite"
          />
          <div
            v-if="activeView === 'photos' ? (hasMoreAlbums || galleryLoadingMore) : (favoriteHasMoreAlbums || favoriteGalleryLoadingMore)"
            class="infinite-loader"
            aria-live="polite"
          >
            <span
              v-if="activeView === 'photos' ? galleryLoadingMore : favoriteGalleryLoadingMore"
              class="loading-spinner small"
              aria-hidden="true"
            ></span>
            <span>
              {{ (activeView === 'photos' ? galleryLoadingMore : favoriteGalleryLoadingMore)
                ? '正在载入下一批完整相簿…'
                : '继续下滑加载更多相簿' }}
            </span>
          </div>
        </div>

        <section v-else class="empty-state">
          <span class="empty-icon">
            <MdIcon name="favoriteBorder" />
          </span>
          <h2>{{ activeView === 'favorites' ? '还没有收藏' : gallerySearch ? '没有匹配的相册' : '图库还是空的' }}</h2>
          <p>{{ activeView === 'favorites' ? '可以收藏单张图片，也可以收藏整个相册。' : gallerySearch ? '请尝试缩短关键词，或使用空格分隔多个关键词。' : '请先等待后端完成照片目录扫描。' }}</p>
          <button v-if="activeView === 'favorites'" class="primary-action" type="button" @click="setView('photos')">浏览最新</button>
          <button v-else-if="gallerySearch" class="primary-action" type="button" @click="clearGallerySearch">清除搜索</button>
        </section>
      </template>

        </div>
      </Transition>
    </main>

    <dialog
      ref="gallerySearchDialog"
      class="person-create-dialog gallery-search-dialog"
      @click="closeGallerySearchFromBackdrop"
      @close="resetGallerySearchInput"
    >
      <div class="person-create-modal gallery-search-modal">
        <header class="person-create-modal-header">
          <div>
            <h2>搜索相册</h2>
            <p>输入相册标题，多个关键词使用空格分隔</p>
          </div>
          <button type="button" aria-label="关闭相册搜索" @click="closeGallerySearchDialog">
            <MdIcon name="close" />
          </button>
        </header>
        <form class="gallery-search" role="search" @submit.prevent="applyGallerySearch">
          <MdIcon name="search" />
          <input
            ref="gallerySearchInputElement"
            v-model="gallerySearchInput"
            type="search"
            placeholder="搜索相册标题"
            autocomplete="off"
            aria-label="搜索相册标题，空格分隔关键词"
          />
          <button
            v-if="gallerySearchInput || gallerySearch"
            class="gallery-search-clear"
            type="button"
            aria-label="清除相册搜索"
            @click="clearGallerySearch"
          >
            <MdIcon name="close" />
          </button>
          <button class="gallery-search-submit" type="submit">搜索</button>
        </form>
      </div>
    </dialog>

    <dialog
      ref="albumPersonDialog"
      class="person-create-dialog album-person-dialog"
      @click="closeAlbumPersonPickerFromBackdrop"
      @close="resetAlbumPersonPicker"
    >
      <div class="person-create-modal album-person-modal">
        <header class="person-create-modal-header">
          <div>
            <h2>关联模特</h2>
            <p>{{ albumPersonTarget?.name }}</p>
          </div>
          <button type="button" aria-label="关闭模特选择" :disabled="personAlbumSavingKeys.size > 0" @click="closeAlbumPersonPicker">
            <MdIcon name="close" />
          </button>
        </header>
        <label class="people-search album-person-search">
          <MdIcon name="search" />
          <input
            v-model="albumPersonSearch"
            type="search"
            placeholder="搜索模特、备注或别名"
            autocomplete="off"
            aria-label="搜索要关联的模特"
          />
          <button v-if="albumPersonSearch" type="button" aria-label="清除搜索" @click="albumPersonSearch = ''">
            <MdIcon name="close" />
          </button>
        </label>
        <div v-if="albumPersonLoading" class="album-person-picker-state" aria-live="polite">
          <span class="loading-spinner small" aria-hidden="true"></span>
          正在读取模特
        </div>
        <p v-else-if="albumPersonError" class="person-create-error" role="alert">{{ albumPersonError }}</p>
        <div v-else-if="albumPersonChoices.length" class="album-person-picker-list">
          <button
            v-for="person in albumPersonChoices"
            :key="person.id"
            type="button"
            :disabled="personAlbumSavingKeys.size > 0"
            @click="associateAlbumPerson(person)"
          >
            <span class="person-avatar" aria-hidden="true">
              <img v-if="person.avatar_url" :src="person.avatar_url" alt="" loading="lazy" fetchpriority="low" />
              <span v-else>{{ person.name.slice(0, 1).toUpperCase() }}</span>
            </span>
            <span>
              <b>{{ person.name }}</b>
              <small v-if="personIdentityLabel(person)">{{ personIdentityLabel(person) }}</small>
                  <small v-if="person.aliases.length">{{ person.aliases.join(' · ') }}</small>
              <small>{{ person.album_count }} 个相册</small>
            </span>
            <span class="album-person-picker-add">+</span>
          </button>
        </div>
        <div v-else class="album-person-picker-state">
          {{ albumPersonSearch ? '没有匹配的模特' : '没有可关联的模特' }}
        </div>
      </div>
    </dialog>

    <dialog
      ref="confirmationDialog"
      class="person-create-dialog confirmation-dialog"
      @cancel.prevent="finishConfirmation(false)"
      @click="closeConfirmationFromBackdrop"
      @close="handleConfirmationClosed"
    >
      <div class="person-create-modal confirmation-modal">
        <header class="person-create-modal-header confirmation-header">
          <div>
            <h2>{{ confirmationTitle }}</h2>
            <p>{{ confirmationMessage }}</p>
          </div>
          <button type="button" aria-label="关闭确认对话框" @click="finishConfirmation(false)">
            <MdIcon name="close" />
          </button>
        </header>
        <div class="confirmation-actions">
          <button type="button" autofocus @click="finishConfirmation(false)">取消</button>
          <button class="confirmation-danger" type="button" @click="finishConfirmation(true)">
            {{ confirmationLabel }}
          </button>
        </div>
      </div>
    </dialog>

    <dialog
      ref="lightbox"
      class="lightbox"
      @click="closeFromBackdrop"
      @close="handleLightboxClose"
      @cancel.prevent="closeLightbox"
      @wheel.prevent="handleLightboxWheel"
    >
      <template v-if="currentPhoto">
        <div
          v-if="slideshowPlaying && currentPhoto.mediaType !== 'video'"
          :key="slideshowProgressKey"
          class="lightbox-progress"
          aria-hidden="true"
        >
          <span :style="{ animationDuration: `${slideshowDelay}ms` }"></span>
        </div>
        <header class="lightbox-header">
          <div class="lightbox-info">
            <span>{{ previewIndex + 1 }} / {{ previewPhotos.length }}</span>
          </div>
          <button v-if="currentPhoto.mediaType === 'image'" class="lightbox-zoom" type="button"
            :aria-label="lightboxZoomed ? '恢复适应屏幕' : '放大图片'" :title="lightboxZoomed ? '恢复适应屏幕' : `放大图片（鼠标单击 / 触屏双击 / ${wheelZoomLabels[wheelZoomModifier]}+滚轮）`"
            :aria-pressed="lightboxZoomed" :disabled="lightboxLoadingPhotoID !== null"
            @click="lightboxSlides?.toggleZoom()">
            <MdIcon :name="lightboxZoomed ? 'zoomOut' : 'zoomIn'" />
          </button>
          <button
            :class="['lightbox-slideshow', { playing: slideshowPlaying }]"
            type="button"
            :aria-label="slideshowPlaying ? '暂停幻灯片' : '播放当前相册幻灯片'"
            :title="slideshowPlaying ? '暂停幻灯片（空格）' : '循环播放当前相册（空格）'"
            @click="toggleSlideshow"
          >
            <MdIcon v-if="slideshowPlaying" name="pause" />
            <MdIcon v-else name="play" />
          </button>
          <button
            :class="['lightbox-favorite', { saved: currentPhoto.favorite }]"
            type="button"
            :aria-label="currentPhoto.favorite ? '取消收藏当前图片' : '收藏当前图片'"
            :title="currentPhoto.favorite ? '取消收藏当前图片（F）' : '收藏当前图片（F）'"
            :aria-pressed="currentPhoto.favorite"
            :disabled="favoriteSavingPhotoIDs.has(currentPhoto.id)"
            @click="togglePhotoFavorite(currentPhoto)"
          >
            <FavoriteIcon :key="currentPhoto.id" :active="currentPhoto.favorite" />
          </button>
          <button class="lightbox-close" type="button" aria-label="关闭预览" @click="closeLightbox">
            <MdIcon name="close" />
          </button>
        </header>
        <LightboxSlides
          ref="lightboxSlides"
          :photos="previewPhotos"
          :index="previewIndex"
          :loading="lightboxLoadingPhotoID !== null"
          :wheel-zoom-modifier="wheelZoomModifier"
          @zoom-gesture="modifierShortcutChorded = true"
          @navigate="stepPhoto"
          @dismiss="closeLightbox"
          @loaded="markLightboxImageLoaded"
          @failed="markLightboxImageFailed"
          @ended="handleVideoEnded"
          @zoom-change="lightboxZoomed = $event; if ($event) slideshowPlaying = false"
        />
      </template>
    </dialog>
  </div>
</template>
