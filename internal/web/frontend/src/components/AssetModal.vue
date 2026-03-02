<script setup lang="ts">
import { ref, watch } from 'vue'
import type { CatalogEntry, AssetKind } from '../types'
import { KIND_LABELS } from '../types'

const props = defineProps<{
  visible: boolean
  kind: AssetKind
  entry: CatalogEntry | null
}>()

const emit = defineEmits<{
  close: []
  save: [kind: AssetKind, entry: CatalogEntry]
}>()

const group = ref('')
const saving = ref(false)

watch(() => props.visible, (v) => {
  if (v && props.entry) {
    group.value = props.entry.group
    saving.value = false
  }
})

function handleSubmit() {
  if (!props.entry) return
  saving.value = true
  emit('save', props.kind, {
    ...props.entry,
    group: group.value.trim(),
  })
}

function stopSaving() {
  saving.value = false
}

defineExpose({ stopSaving })
</script>

<template>
  <Transition name="modal">
    <div
      v-if="visible && entry"
      class="fixed inset-0 z-50 flex items-center justify-center p-4"
      @mousedown.self="emit('close')"
    >
      <div class="fixed inset-0 bg-black/40" aria-hidden="true" />
      <div
        class="modal-panel relative bg-white rounded-xl shadow-xl w-full max-w-md"
        role="dialog"
        aria-modal="true"
      >
        <div class="flex items-center justify-between px-6 pt-5 pb-0">
          <h2 class="text-lg font-semibold text-slate-900">Edit Asset</h2>
          <button
            class="p-1.5 rounded-lg text-slate-400 hover:text-slate-600 hover:bg-slate-100
                   cursor-pointer focus:outline-none focus:ring-2 focus:ring-blue-500
                   transition-colors duration-200"
            aria-label="Close dialog"
            @click="emit('close')"
          >
            <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </button>
        </div>

        <form class="px-6 py-5 space-y-4" @submit.prevent="handleSubmit">
          <div class="grid grid-cols-[80px_1fr] gap-y-2 text-sm">
            <span class="text-slate-500 font-medium">Type</span>
            <span class="text-slate-900">{{ KIND_LABELS[kind] }}</span>

            <span class="text-slate-500 font-medium">Name</span>
            <span class="text-slate-900 font-mono">{{ entry.name }}</span>

            <span class="text-slate-500 font-medium">Source</span>
            <span class="text-slate-900 font-mono text-xs break-all">{{ entry.source || '(none)' }}</span>

            <span class="text-slate-500 font-medium">Desc</span>
            <span class="text-slate-900">{{ entry.description || '(none)' }}</span>
          </div>

          <div class="pt-2 border-t border-slate-100">
            <label for="edit-group" class="block text-sm font-medium text-slate-700 mb-1">Group</label>
            <input
              id="edit-group"
              v-model="group"
              type="text"
              placeholder="e.g. Code Quality (default: Ungrouped)"
              class="w-full px-3 py-2 text-sm border border-slate-200 rounded-lg
                     focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent
                     placeholder:text-slate-400 transition-colors duration-200"
            >
          </div>

          <div class="flex justify-end gap-3 pt-2">
            <button
              type="button"
              class="px-4 py-2 text-sm font-medium text-slate-700 bg-slate-100 hover:bg-slate-200
                     rounded-lg cursor-pointer focus:outline-none focus:ring-2 focus:ring-blue-500
                     transition-colors duration-200"
              @click="emit('close')"
            >Cancel</button>
            <button
              type="submit"
              :disabled="saving"
              class="px-4 py-2 text-sm font-medium text-white bg-blue-500 hover:bg-blue-600
                     rounded-lg cursor-pointer focus:outline-none focus:ring-2 focus:ring-blue-500
                     focus:ring-offset-2 transition-colors duration-200
                     disabled:opacity-50 disabled:cursor-not-allowed"
            >{{ saving ? 'Saving...' : 'Save' }}</button>
          </div>
        </form>
      </div>
    </div>
  </Transition>
</template>
