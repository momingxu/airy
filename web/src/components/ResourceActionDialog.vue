<script setup lang="ts">
import { ref } from 'vue'
import MdIcon from './MdIcon.vue'
defineProps<{ title: string; icon: string; wide?: boolean }>()
const dialog = ref<HTMLDialogElement>()
const opened = ref(false)
function show() {
 opened.value = true
 dialog.value?.showModal()
}
</script>
<template>
 <button class="secondary-action resource-action-trigger" type="button" @click="show"><MdIcon :name="icon" />{{ title }}</button>
 <dialog ref="dialog" :aria-label="title" :class="['resource-action-dialog', { wide }]" @click="event => { if (event.target === dialog) dialog?.close() }">
  <header class="resource-action-header"><h2>{{ title }}</h2><button class="resource-action-close" type="button" aria-label="关闭窗口" @click="dialog?.close()"><MdIcon name="close" /></button></header>
  <div v-if="opened" class="resource-action-body"><slot /></div>
 </dialog>
</template>
<style scoped>
.resource-action-trigger{display:inline-flex;align-items:center;gap:10px}.resource-action-trigger :deep(svg){width:22px;height:22px}.resource-action-dialog{width:min(680px,calc(100vw - 32px));max-width:none;max-height:calc(100dvh - 48px);padding:0;border:1px solid var(--outline-variant,#ccc);border-radius:24px;background:var(--surface,#fafaf5);color:var(--on-surface,#20251d);box-shadow:0 24px 80px #0003;overflow:auto}.resource-action-dialog.wide{width:min(1100px,calc(100vw - 32px))}.resource-action-dialog::backdrop{background:#0006}.resource-action-header{display:flex;align-items:center;justify-content:space-between;gap:20px;padding:20px 24px;border-bottom:1px solid var(--outline-variant,#ddd)}.resource-action-header h2{font-size:22px;margin:0}.resource-action-close{display:grid;place-items:center;background:transparent;color:inherit;border:0;border-radius:50%;width:40px;height:40px;cursor:pointer}.resource-action-close :deep(svg){width:24px;height:24px}.resource-action-close:hover{background:#8882}.resource-action-body{padding:24px}.resource-action-body :deep(.folder-browser){border:0;padding:0;background:transparent}.resource-action-body :deep(.folder-browser > header){display:none}.resource-action-body :deep(.folder-manager-toolbar){margin:0}.resource-action-body :deep(.global-scan-toolbar){display:flex;flex-direction:column;align-items:stretch}.resource-action-body :deep(.global-scan-actions){justify-content:flex-end}@media(max-width:600px){.resource-action-body{padding:16px}.resource-action-header{padding:16px}}
</style>
