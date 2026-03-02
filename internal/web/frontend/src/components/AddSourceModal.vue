<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import type { DiscoveredItem } from '../types'
import * as api from '../api'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ close: []; added: [count: number] }>()

const step = ref<'source' | 'select'>('source')
const sourceInput = ref('')
const resolvedSource = ref('')
const discoveredItems = ref<DiscoveredItem[]>([])
const selectedKeys = ref<Set<string>>(new Set())
const group = ref('')
const loading = ref(false)
const saving = ref(false)
const error = ref('')

const KIND_LABELS: Record<string, string> = {
  skill: 'Skill',
  rule: 'Rule',
  mcp: 'MCP',
  command: 'Command',
}

watch(() => props.visible, (v) => {
  if (v) {
    step.value = 'source'
    sourceInput.value = ''
    resolvedSource.value = ''
    discoveredItems.value = []
    selectedKeys.value = new Set()
    group.value = ''
    loading.value = false
    saving.value = false
    error.value = ''
  }
})

function itemKey(item: DiscoveredItem) {
  return `${item.kind}:${item.name}`
}

function toggleItem(item: DiscoveredItem) {
  const key = itemKey(item)
  const next = new Set(selectedKeys.value)
  if (next.has(key)) {
    next.delete(key)
  } else {
    next.add(key)
  }
  selectedKeys.value = next
}

const allSelected = computed(() =>
  discoveredItems.value.length > 0 && selectedKeys.value.size === discoveredItems.value.length
)

function toggleAll() {
  if (allSelected.value) {
    selectedKeys.value = new Set()
  } else {
    selectedKeys.value = new Set(discoveredItems.value.map(itemKey))
  }
}

async function handleDiscover() {
  const src = sourceInput.value.trim()
  if (!src) return
  loading.value = true
  error.value = ''
  try {
    const result = await api.discoverAssets(src)
    resolvedSource.value = result.source
    discoveredItems.value = result.items ?? []
    if (discoveredItems.value.length === 0) {
      error.value = 'No assets found in this source.'
    } else {
      selectedKeys.value = new Set(discoveredItems.value.map(itemKey))
      step.value = 'select'
    }
  } catch (err: unknown) {
    error.value = err instanceof Error ? err.message : 'Failed to discover assets'
  } finally {
    loading.value = false
  }
}

async function handleAdd() {
  const selected = discoveredItems.value.filter(i => selectedKeys.value.has(itemKey(i)))
  if (selected.length === 0) return
  saving.value = true
  try {
    await api.batchAddAssets(resolvedSource.value, group.value.trim() || 'Ungrouped', selected)
    emit('added', selected.length)
  } catch (err: unknown) {
    error.value = err instanceof Error ? err.message : 'Failed to add assets'
    saving.value = false
  }
}
</script>

<template>
  <Transition name="modal">
    <div
      v-if="visible"
      class="fixed inset-0 z-50 flex items-center justify-center p-4"
      @mousedown.self="emit('close')"
    >
      <div class="fixed inset-0 bg-black/40" aria-hidden="true" />
      <div
        class="modal-panel relative bg-white rounded-xl shadow-xl w-full max-w-lg max-h-[80vh] flex flex-col"
        role="dialog"
        aria-modal="true"
      >
        <div class="flex items-center justify-between px-6 pt-5 pb-0 shrink-0">
          <h2 class="text-lg font-semibold text-slate-900">Add Asset</h2>
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

        <!-- Step 1: Source Input -->
        <div v-if="step === 'source'" class="px-6 py-5 space-y-4">
          <p class="text-sm text-slate-600">
            Enter a GitHub repo (e.g. <code class="text-xs bg-slate-100 px-1 py-0.5 rounded">user/repo</code>),
            Git URL, or local path to discover assets.
          </p>
          <form @submit.prevent="handleDiscover" class="space-y-4">
            <div>
              <label for="source-input" class="block text-sm font-medium text-slate-700 mb-1">Source</label>
              <input
                id="source-input"
                v-model="sourceInput"
                type="text"
                required
                :disabled="loading"
                placeholder="e.g. silenceper/ai-assets or /path/to/project"
                class="w-full px-3 py-2 text-sm font-mono border border-slate-200 rounded-lg
                       focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent
                       placeholder:text-slate-400 transition-colors duration-200 disabled:opacity-60"
              >
            </div>

            <p v-if="error" class="text-sm text-red-500">{{ error }}</p>

            <div class="flex justify-end gap-3">
              <button
                type="button"
                class="px-4 py-2 text-sm font-medium text-slate-700 bg-slate-100 hover:bg-slate-200
                       rounded-lg cursor-pointer transition-colors duration-200"
                @click="emit('close')"
              >Cancel</button>
              <button
                type="submit"
                :disabled="loading || !sourceInput.trim()"
                class="inline-flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-blue-500 hover:bg-blue-600
                       rounded-lg cursor-pointer focus:outline-none focus:ring-2 focus:ring-blue-500
                       focus:ring-offset-2 transition-colors duration-200
                       disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <svg v-if="loading" class="animate-spin w-4 h-4" viewBox="0 0 24 24" fill="none">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
                </svg>
                {{ loading ? 'Discovering...' : 'Discover' }}
              </button>
            </div>
          </form>
        </div>

        <!-- Step 2: Select Discovered Assets -->
        <div v-else class="flex flex-col min-h-0 flex-1">
          <div class="px-6 pt-4 pb-2 shrink-0">
            <div class="flex items-center justify-between mb-3">
              <p class="text-sm text-slate-600">
                Found <strong>{{ discoveredItems.length }}</strong> asset(s) from
                <code class="text-xs bg-slate-100 px-1 py-0.5 rounded font-mono">{{ resolvedSource }}</code>
              </p>
              <button
                class="text-xs text-blue-500 hover:text-blue-600 cursor-pointer font-medium"
                @click="step = 'source'"
              >Change source</button>
            </div>

            <label class="flex items-center gap-2 py-1.5 cursor-pointer select-none">
              <input
                type="checkbox"
                :checked="allSelected"
                @change="toggleAll"
                class="w-4 h-4 rounded border-slate-300 text-blue-500 focus:ring-blue-500 cursor-pointer"
              >
              <span class="text-sm font-medium text-slate-700">Select all</span>
            </label>
          </div>

          <div class="overflow-y-auto flex-1 px-6 border-t border-slate-100">
            <div
              v-for="item in discoveredItems"
              :key="itemKey(item)"
              class="flex items-start gap-3 py-3 border-b border-slate-100 last:border-b-0"
            >
              <input
                type="checkbox"
                :checked="selectedKeys.has(itemKey(item))"
                @change="toggleItem(item)"
                class="mt-0.5 w-4 h-4 rounded border-slate-300 text-blue-500 focus:ring-blue-500 cursor-pointer shrink-0"
              >
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2">
                  <span class="font-medium text-sm text-slate-900">{{ item.name }}</span>
                  <span
                    class="px-1.5 py-0.5 text-xs font-medium rounded"
                    :class="{
                      'bg-blue-50 text-blue-600': item.kind === 'skill',
                      'bg-green-50 text-green-600': item.kind === 'rule',
                      'bg-yellow-50 text-yellow-600': item.kind === 'mcp',
                      'bg-purple-50 text-purple-600': item.kind === 'command',
                    }"
                  >{{ KIND_LABELS[item.kind] ?? item.kind }}</span>
                </div>
                <p v-if="item.desc" class="text-xs text-slate-500 mt-0.5 line-clamp-2">{{ item.desc }}</p>
              </div>
            </div>
          </div>

          <div class="px-6 py-4 border-t border-slate-200 shrink-0 space-y-3">
            <div>
              <label for="batch-group" class="block text-sm font-medium text-slate-700 mb-1">Group</label>
              <input
                id="batch-group"
                v-model="group"
                type="text"
                placeholder="e.g. Code Quality (default: Ungrouped)"
                class="w-full px-3 py-2 text-sm border border-slate-200 rounded-lg
                       focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent
                       placeholder:text-slate-400 transition-colors duration-200"
              >
            </div>
            <p v-if="error" class="text-sm text-red-500">{{ error }}</p>
            <div class="flex justify-end gap-3">
              <button
                type="button"
                class="px-4 py-2 text-sm font-medium text-slate-700 bg-slate-100 hover:bg-slate-200
                       rounded-lg cursor-pointer transition-colors duration-200"
                @click="emit('close')"
              >Cancel</button>
              <button
                :disabled="saving || selectedKeys.size === 0"
                class="px-4 py-2 text-sm font-medium text-white bg-blue-500 hover:bg-blue-600
                       rounded-lg cursor-pointer focus:outline-none focus:ring-2 focus:ring-blue-500
                       focus:ring-offset-2 transition-colors duration-200
                       disabled:opacity-50 disabled:cursor-not-allowed"
                @click="handleAdd"
              >
                {{ saving ? 'Adding...' : `Add ${selectedKeys.size} Asset(s)` }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </Transition>
</template>
