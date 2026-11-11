<script setup lang="ts">
import { computed, ref } from 'vue'
import MdIcon from './MdIcon.vue'

const emit = defineEmits<{ applied: [] }>()
interface PreviewRow {
  old_series: string; series: string; old_people: string[]; people: string[]; create_series: boolean; new_people: string[]
  line: number; id: number; old_title: string; title: string
  old_date: string; date: string; error: string; changed: boolean
}
interface Preview { rows: PreviewRow[]; changes: number; valid: boolean; token: string }
const dialog = ref<HTMLDialogElement | null>(null)
const busy = ref(false)
const isOpen = ref(false)
const error = ref('')
const message = ref('')
const preview = ref<Preview | null>(null)
const fileName = ref('')
const csv = ref('')
const page = ref(1)
const pageCount = computed(() => Math.max(1, Math.ceil((preview.value?.rows.length ?? 0) / 100)))
const visibleRows = computed(() => preview.value?.rows.slice((page.value - 1) * 100, page.value * 100) ?? [])
const creations = computed(() => {
  const series = new Set(preview.value?.rows.filter(row => row.create_series).map(row => row.series))
  const people = new Set(preview.value?.rows.flatMap(row => row.new_people))
  return { series: series.size, people: people.size }
})
const errorCount = computed(() => preview.value?.rows.filter(row => row.error).length ?? 0)

async function checkedFetch(url: string, init?: RequestInit) {
  const response = await fetch(url, init)
  if (response.status === 401) window.dispatchEvent(new Event('airy:unauthorized'))
  if (!response.ok) {
    const body = await response.json().catch(() => null)
    throw new Error(body?.error ?? `请求失败（${response.status}）`)
  }
  return response
}
function openImport() {
  preview.value = null; error.value = ''; message.value = ''; csv.value = ''; fileName.value = ''; page.value = 1
  dialog.value?.showModal()
  isOpen.value = true
}
function close() { if (!busy.value) { dialog.value?.close(); isOpen.value = false } }
async function exportCsv() {
  busy.value = true; error.value = ''; message.value = ''
  try {
    const response = await checkedFetch('/api/albums.csv')
    const url = URL.createObjectURL(await response.blob())
    const link = document.createElement('a')
    link.href = url; link.download = 'airy-albums.csv'
    document.body.append(link); link.click(); link.remove()
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    message.value = 'CSV 已导出'
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '导出失败' }
  finally { busy.value = false }
}
async function chooseFile(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]; input.value = ''
  if (!file) return
  preview.value = null; csv.value = ''; error.value = ''; message.value = ''; page.value = 1; fileName.value = file.name
  if (file.size > 5 * 1024 * 1024) { error.value = 'CSV 文件不能超过 5 MB'; return }
  busy.value = true
  try {
    csv.value = new TextDecoder('utf-8', { fatal: true }).decode(await file.arrayBuffer())
    const response = await checkedFetch('/api/albums/csv-preview', {
      method: 'POST', headers: { 'Content-Type': 'text/csv; charset=utf-8' }, body: csv.value,
    })
    preview.value = await response.json() as Preview
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '读取 CSV 失败，请使用 UTF-8 编码' }
  finally { busy.value = false }
}
async function apply() {
  if (!preview.value?.valid || !preview.value.changes || busy.value) return
  busy.value = true; error.value = ''
  try {
    const response = await checkedFetch('/api/albums/csv-import', {
      method: 'POST', headers: { 'Content-Type': 'text/csv; charset=utf-8', 'X-Preview-Token': preview.value.token }, body: csv.value,
    })
    const result = await response.json() as Preview
    message.value = `已修改 ${result.changes} 个相册`
    preview.value = null; csv.value = ''; emit('applied')
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '保存失败'
    preview.value = null // Require a fresh preview before retrying.
  } finally { busy.value = false }
}
</script>

<template>
  <div class="album-csv-actions">
    <button class="scan-button" type="button" :disabled="busy" @click="exportCsv">导出 CSV</button>
    <button class="scan-button" type="button" :disabled="busy" @click="openImport">导入 CSV</button>
    <span v-if="message && !isOpen" class="csv-status" role="status">{{ message }}</span>
    <span v-if="error && !isOpen" class="csv-error" role="alert">{{ error }}</span>
    <dialog ref="dialog" class="person-create-dialog album-csv-dialog" @cancel.prevent="close" @click="event => { if (event.target === dialog) close() }">
      <div class="person-create-modal">
        <header class="person-create-modal-header">
          <div><h2>导入相册 CSV</h2><p>全部资源库 · 仅按相册 ID 匹配</p></div>
          <button type="button" :disabled="busy" aria-label="关闭 CSV 导入" @click="close"><MdIcon name="close" /></button>
        </header>
        <p class="csv-help">先导出 CSV，再修改相册标题、发布日期、系列和模特。请保留表头与相册 ID；日期使用 YYYY-MM-DD，留空则保留原值。文件为 UTF-8 编码，最多 5 MB、20000 行。</p>
        <p class="csv-help">系列填写一个名称；多个模特名称用分号分隔。名称不存在时将在应用修改时创建。同名模特无法唯一匹配时会报错。系列或模特留空保留原关联，填写名称替换关联，填写 [清空] 解除关联。旧版三列 CSV 仍可导入。</p>
        <label class="csv-file-label">选择修改后的 CSV
          <input type="file" accept=".csv,text/csv" :disabled="busy" @change="chooseFile" />
        </label>
        <p v-if="fileName" class="csv-help">{{ fileName }}</p>
        <p v-if="busy" role="status">正在处理，请稍候…</p>
        <p v-if="error" class="csv-error" role="alert">{{ error }}</p>
        <p v-if="message" class="csv-status" role="status">{{ message }}</p>
        <template v-if="preview">
          <p role="status">共 {{ preview.rows.length }} 行，{{ preview.changes }} 项变更，{{ errorCount }} 行错误。</p>
          <p v-if="creations.series || creations.people" class="csv-help">应用时将新建 {{ creations.series }} 个系列、{{ creations.people }} 个模特。</p>
          <p v-if="!preview.valid" class="csv-error">请修正所有错误后重新选择文件，当前不会保存任何修改。</p>
          <div class="csv-table-scroll" tabindex="0" aria-label="CSV 修改预览">
            <table class="csv-table">
              <thead><tr><th>行 / ID</th><th>原标题 → 新标题</th><th>原日期 → 新日期</th><th>原系列 → 新系列</th><th>原模特 → 新模特</th><th>状态</th></tr></thead>
              <tbody><tr v-for="row in visibleRows" :key="row.line" :class="{ 'csv-invalid': row.error }">
                <td>{{ row.line }} / {{ row.id || '无效' }}</td>
                <td><span class="csv-old">{{ row.old_title || '—' }}</span><span>{{ row.title }}</span></td>
                <td><span class="csv-old">{{ row.old_date || '—' }}</span><span>{{ row.date || '—' }}</span></td>
                <td><span class="csv-old">{{ row.old_series || '—' }}</span><span>{{ row.series || '—' }}{{ row.create_series ? '（新建）' : '' }}</span></td>
                <td><span class="csv-old">{{ row.old_people.join('；') || '—' }}</span><span>{{ row.people.join('；') || '—' }}</span><small v-if="row.new_people.length">新建：{{ row.new_people.join('；') }}</small></td>
                <td>{{ row.error || (row.changed ? '待修改' : '不变') }}</td>
              </tr></tbody>
            </table>
          </div>
          <div v-if="pageCount > 1" class="csv-pagination">
            <button class="scan-button" :disabled="page === 1" @click="page--">上一页</button>
            <span>{{ page }} / {{ pageCount }}</span>
            <button class="scan-button" :disabled="page === pageCount" @click="page++">下一页</button>
          </div>
        </template>
        <div class="csv-footer">
          <button class="scan-button" type="button" :disabled="busy" @click="close">关闭</button>
          <button class="primary-action" type="button" :disabled="busy || !preview?.valid || !preview?.changes" @click="apply">应用修改{{ preview?.changes ? `（${preview.changes}）` : '' }}</button>
        </div>
      </div>
    </dialog>
  </div>
</template>

<style scoped>
.album-csv-actions { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
.album-csv-dialog { width: min(1040px, calc(100vw - 24px)); }
.csv-help { color: var(--text-muted); font-size: 12px; line-height: 1.8; }
.csv-file-label { display: grid; gap: 8px; margin: 16px 0; font-size: 13px; }
.csv-error { color: var(--error); font-size: 12px; overflow-wrap: anywhere; }
.csv-status { color: var(--accent-strong); font-size: 12px; }
.csv-table-scroll { overflow: auto; max-height: 44vh; margin: 16px 0; border: 1px solid var(--line); border-radius: 12px; }
.csv-table { border-collapse: collapse; width: 100%; font-size: 12px; min-width: 900px; }
.csv-table th { position: sticky; top: 0; background: var(--surface-muted); text-align: left; }
.csv-table td, .csv-table th { padding: 10px; border-bottom: 1px solid var(--line); vertical-align: top; }
.csv-table td { white-space: pre-wrap; overflow-wrap: anywhere; max-width: 360px; }
.csv-table td span { display: block; }
.csv-old { color: var(--text-muted); margin-bottom: 6px; }
.csv-invalid { background: color-mix(in srgb, var(--error) 8%, var(--surface)); }
.csv-pagination, .csv-footer { display: flex; gap: 12px; align-items: center; justify-content: flex-end; margin-top: 16px; }
.csv-footer .primary-action { padding: 10px 20px; }
</style>
