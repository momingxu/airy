<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { createDirectory } from '../api/directories'
import MdIcon from './MdIcon.vue'
const emit = defineEmits<{ added: [] }>()
interface Folder { name: string; path: string; modified_at: string }
const path = ref('/mnt'), keyword = ref(''), error = ref(''), message = ref('')
const folders = ref<Folder[]>([]), selected = ref<string[]>([])
const loading = ref(false), saving = ref(false), page = ref(1), depth = ref(1)
const filtered = computed(() => folders.value.filter(f => f.name.toLowerCase().includes(keyword.value.toLowerCase())))
const pages = computed(() => Math.max(1, Math.ceil(filtered.value.length / 10)))
const visible = computed(() => filtered.value.slice((page.value - 1) * 10, page.value * 10))
const all = computed(() => visible.value.length > 0 && visible.value.every(f => selected.value.includes(f.path)))
const crumbs = computed(() => path.value.split('/').filter(Boolean).map((name, i, parts) => ({ name, path: '/' + parts.slice(0, i + 1).join('/') })))
async function browse(target: string) {
 if (loading.value || saving.value) return
 loading.value = true; error.value = ''
 try {
  const response = await fetch('/api/directories/browse?path=' + encodeURIComponent(target))
  if (response.status === 401) window.dispatchEvent(new Event('airy:unauthorized'))
  const body = await response.json()
  if (!response.ok) throw new Error(body.error || '读取目录失败')
  folders.value = body.directories; path.value = body.path
  selected.value = []; keyword.value = ''; page.value = 1
 } catch (e) { error.value = e instanceof Error ? e.message : '读取目录失败' }
 finally { loading.value = false }
}
function toggleAll() {
 const paths = visible.value.map(f => f.path)
 selected.value = all.value ? selected.value.filter(p => !paths.includes(p)) : [...new Set([...selected.value, ...paths])]
}
async function add(paths: string[]) {
 if (saving.value || !paths.length) return
 saving.value = true; error.value = ''; message.value = ''
 let created = 0
 try {
  for (const target of paths) {
   const result = await createDirectory(target, depth.value)
   created += result.created
   selected.value = selected.value.filter(p => p !== target)
  }
  message.value = `已添加 ${created} 个资源库。`
 } catch (e) { error.value = `已添加 ${created} 个资源库。` + (e instanceof Error ? e.message : '添加失败') }
 finally { saving.value = false; emit('added') }
}
onMounted(() => browse('/mnt'))
</script>
<template>
 <div class="folder-browser">
  <header><h2>文件夹管理</h2><button :disabled="loading || saving" @click="browse(path)">刷新</button></header>
  <nav aria-label="目录路径"><template v-for="(crumb, i) in crumbs" :key="crumb.path"><span v-if="i"> / </span><button :disabled="loading || saving" @click="browse(crumb.path)">{{ crumb.name }}</button></template></nav>
  <div class="browser-toolbar">
   <label class="browser-search"><MdIcon name="search" /><input v-model="keyword" placeholder="关键字过滤" aria-label="筛选文件夹" @input="page = 1" /></label>
   <span>已选中 {{ selected.length }} 个目录</span>
   <label>相册深度 <select v-model.number="depth" :disabled="saving"><option v-for="n in 10" :key="n" :value="n">{{ n }} 级</option></select></label>
   <button class="primary-action" :disabled="!selected.length || saving || loading" @click="add([...selected])">{{ saving ? '正在添加…' : '添加所选' }}</button>
  </div>
  <p v-if="error" class="inline-error" role="alert">{{ error }} <button @click="browse(path)">重试</button></p><p v-if="message" role="status">{{ message }}</p>
  <div class="browser-table-wrap" :aria-busy="loading"><table>
   <thead><tr><th><input type="checkbox" aria-label="选择当前页" :checked="all" :disabled="loading || saving" @change="toggleAll" /></th><th>名称</th><th>修改时间</th><th>文件大小</th><th>操作</th></tr></thead>
   <tbody><tr v-for="folder in visible" :key="folder.path">
    <td><input v-model="selected" type="checkbox" :value="folder.path" :aria-label="`选择 ${folder.name}`" :disabled="saving || loading" /></td>
    <td><button class="browser-folder" :disabled="loading || saving" @click="browse(folder.path)"><MdIcon name="folder" />{{ folder.name }}</button></td>
    <td>{{ new Date(folder.modified_at).toLocaleString('zh-CN', { hour12: false }) }}</td><td>—</td>
    <td><button :aria-label="`添加 ${folder.name}`" :disabled="saving || loading" @click="add([folder.path])"><MdIcon name="add" /></button></td>
   </tr><tr v-if="loading || !visible.length"><td colspan="5" class="browser-empty">{{ loading ? '正在读取目录…' : '没有可显示的文件夹' }}</td></tr></tbody>
  </table></div>
  <footer><button :disabled="page <= 1" @click="page--">← 上一页</button><span>{{ page }} / {{ pages }}</span><button :disabled="page >= pages" @click="page++">下一页 →</button></footer>
  <p class="browser-hint">仅浏览 /mnt 内的文件夹。点击名称进入下级目录，点击 + 或勾选后添加为资源库；添加后可在下方扫描。</p>
 </div>
</template>
<style scoped>
.folder-browser{background:var(--surface-container-lowest,#fff);border:1px solid var(--outline-variant,#ddd);border-radius:24px;padding:28px;min-width:0}header,.browser-toolbar,footer{display:flex;align-items:center;gap:16px;flex-wrap:wrap}header,footer{justify-content:space-between}h2{margin:0;font-size:24px}nav{margin:20px 0}.folder-browser button{cursor:pointer}.folder-browser button:disabled{cursor:default;opacity:.5}header button,nav button,table button,footer button{background:transparent;border:0;color:inherit;padding:8px}.browser-toolbar{margin:24px 0;font-size:14px}.browser-toolbar select{padding:8px;border:1px solid #ccc;border-radius:8px;background:transparent;color:inherit}.browser-search{display:flex;align-items:center;gap:8px;border:1px solid #ccc;border-radius:10px;padding:10px;flex:1;min-width:150px}.browser-search input{border:0;outline:none;background:transparent;color:inherit;min-width:0;width:100%}.folder-browser :deep(svg){width:22px;height:22px;flex-shrink:0}.browser-table-wrap{overflow:auto;max-height:560px}table{width:100%;border-collapse:collapse;white-space:nowrap;text-align:left;font-size:14px}th{font-weight:400;color:var(--on-surface-variant,#777)}td,th{padding:16px 10px;border-bottom:1px solid var(--outline-variant,#eee)}th:nth-child(2){width:100%}table input{width:18px;height:18px;accent-color:var(--primary,#456b32)}.browser-folder{display:flex;align-items:center;gap:12px;text-align:left;font:inherit}footer{margin-top:20px}.browser-empty{text-align:center;padding:40px}.browser-hint{font-size:13px;line-height:1.7;opacity:.7;margin-bottom:0}@media(max-width:600px){.folder-browser{padding:16px}.browser-search{flex-basis:100%}}
</style>
