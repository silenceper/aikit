<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import type { CatalogConfig, CatalogEntry, AssetKind, FlatEntry } from './types'
import { KINDS } from './types'
import * as api from './api'
import AssetModal from './components/AssetModal.vue'
import AddSourceModal from './components/AddSourceModal.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'

const catalog = ref<CatalogConfig>({})
const loading = ref(true)
const search = ref('')
const activeTab = ref<'all' | AssetKind>('all')
const activeGroup = ref<string | null>(null)

interface Toast { id: number; message: string; type: 'success' | 'error' }
const toasts = ref<Toast[]>([])
let toastId = 0

function showToast(message: string, type: 'success' | 'error' = 'success') {
  const id = ++toastId
  toasts.value.push({ id, message, type })
  setTimeout(() => {
    toasts.value = toasts.value.filter(t => t.id !== id)
  }, 3000)
}

const allEntries = computed<FlatEntry[]>(() => {
  const result: FlatEntry[] = []
  for (const kind of KINDS) {
    const entries = catalog.value[kind] ?? []
    for (const e of entries) {
      result.push({ ...e, kind })
    }
  }
  return result
})

const filteredByTab = computed(() => {
  let entries = allEntries.value
  if (activeTab.value !== 'all') {
    entries = entries.filter(e => e.kind === activeTab.value)
  }
  const q = search.value.toLowerCase().trim()
  if (q) {
    entries = entries.filter(e =>
      e.name.toLowerCase().includes(q) ||
      e.description.toLowerCase().includes(q) ||
      e.source.toLowerCase().includes(q) ||
      e.group.toLowerCase().includes(q)
    )
  }
  return entries
})

const groupNames = computed(() => {
  const set = new Set<string>()
  for (const e of filteredByTab.value) {
    set.add(e.group || 'Ungrouped')
  }
  return Array.from(set).sort((a, b) => {
    if (a === 'Ungrouped') return 1
    if (b === 'Ungrouped') return -1
    return a.localeCompare(b)
  })
})

const displayEntries = computed(() => {
  if (!activeGroup.value) return filteredByTab.value
  return filteredByTab.value.filter(e => (e.group || 'Ungrouped') === activeGroup.value)
})

const counts = computed(() => {
  const c: Record<string, number> = { all: allEntries.value.length }
  for (const kind of KINDS) {
    c[kind] = (catalog.value[kind] ?? []).length
  }
  return c
})

const groupCounts = computed(() => {
  const c: Record<string, number> = {}
  for (const e of filteredByTab.value) {
    const g = e.group || 'Ungrouped'
    c[g] = (c[g] || 0) + 1
  }
  return c
})

// Add Source modal
const addModalVisible = ref(false)

function openAddModal() {
  addModalVisible.value = true
}

async function handleAdded(count: number) {
  addModalVisible.value = false
  showToast(`Added ${count} asset(s)`)
  await loadCatalog()
}

// Edit modal
const editModalVisible = ref(false)
const editModalKind = ref<AssetKind>('skills')
const editModalEntry = ref<CatalogEntry | null>(null)
const editModalRef = ref<InstanceType<typeof AssetModal> | null>(null)

function openEditModal(entry: FlatEntry) {
  editModalKind.value = entry.kind
  editModalEntry.value = { name: entry.name, source: entry.source, description: entry.description, group: entry.group }
  editModalVisible.value = true
}

async function handleSave(kind: AssetKind, entry: CatalogEntry) {
  try {
    await api.updateEntry(kind, entry.name, entry)
    showToast(`Updated ${entry.name}`)
    editModalVisible.value = false
    await loadCatalog()
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : 'Unknown error'
    showToast(msg, 'error')
    editModalRef.value?.stopSaving()
  }
}

// Confirm dialog
const confirmVisible = ref(false)
const confirmTarget = ref<FlatEntry | null>(null)

function openDeleteConfirm(entry: FlatEntry) {
  confirmTarget.value = entry
  confirmVisible.value = true
}

async function handleDelete() {
  const target = confirmTarget.value
  if (!target) return
  confirmVisible.value = false
  try {
    await api.deleteEntry(target.kind, target.name)
    showToast(`Deleted ${target.name}`)
    await loadCatalog()
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : 'Unknown error'
    showToast(msg, 'error')
  }
}

async function loadCatalog() {
  try {
    catalog.value = await api.getCatalog()
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : 'Failed to load catalog'
    showToast(msg, 'error')
  }
}

function setActiveTab(tab: 'all' | AssetKind) {
  activeTab.value = tab
  activeGroup.value = null
}

onMounted(async () => {
  await loadCatalog()
  loading.value = false
})
</script>

<template>
  <div class="bg-slate-50 text-slate-900 min-h-screen antialiased">
    <!-- Top Bar -->
    <header class="sticky top-0 z-10 bg-white/95 backdrop-blur border-b border-slate-200">
      <div class="max-w-5xl mx-auto px-4 sm:px-6 h-16 flex items-center gap-4">
        <div class="flex items-center gap-2 shrink-0">
          <svg class="w-7 h-7 text-blue-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
            <polyline points="3.27 6.96 12 12.01 20.73 6.96"/>
            <line x1="12" y1="22.08" x2="12" y2="12"/>
          </svg>
          <h1 class="text-lg font-semibold text-slate-900 hidden sm:block">AIKIT Catalog Manager</h1>
        </div>

        <div class="relative flex-1 max-w-md">
          <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-400 pointer-events-none" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>
          </svg>
          <input
            v-model="search"
            type="search"
            placeholder="Search assets..."
            class="w-full pl-9 pr-3 py-2 text-sm bg-slate-50 border border-slate-200 rounded-lg
                   focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent
                   placeholder:text-slate-400 transition-colors duration-200"
          >
        </div>

        <button
          class="inline-flex items-center gap-1.5 px-4 py-2 text-sm font-medium text-white
                 bg-blue-500 hover:bg-blue-600 rounded-lg cursor-pointer
                 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2
                 transition-colors duration-200 shrink-0"
          @click="openAddModal"
        >
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>
          </svg>
          <span class="hidden sm:inline">Add Asset</span>
        </button>
      </div>
    </header>

    <!-- Kind Tabs -->
    <nav class="bg-white border-b border-slate-200">
      <div class="max-w-5xl mx-auto px-4 sm:px-6 flex gap-1 overflow-x-auto">
        <button
          v-for="tab in (['all', ...KINDS] as const)"
          :key="tab"
          class="tab-btn"
          :class="{ active: activeTab === tab }"
          @click="setActiveTab(tab)"
        >
          {{ tab === 'all' ? 'All' : tab === 'mcps' ? 'MCPs' : tab.charAt(0).toUpperCase() + tab.slice(1) }}
          <span class="tab-badge">{{ counts[tab] ?? 0 }}</span>
        </button>
      </div>
    </nav>

    <!-- Group Sub-tabs -->
    <div v-if="groupNames.length > 1" class="bg-white border-b border-slate-100">
      <div class="max-w-5xl mx-auto px-4 sm:px-6 flex gap-1 overflow-x-auto py-1">
        <button
          class="group-tab"
          :class="{ active: activeGroup === null }"
          @click="activeGroup = null"
        >
          All Groups
          <span class="group-tab-badge">{{ filteredByTab.length }}</span>
        </button>
        <button
          v-for="g in groupNames"
          :key="g"
          class="group-tab"
          :class="{ active: activeGroup === g }"
          @click="activeGroup = g"
        >
          {{ g }}
          <span class="group-tab-badge">{{ groupCounts[g] ?? 0 }}</span>
        </button>
      </div>
    </div>

    <!-- Main -->
    <main class="max-w-5xl mx-auto px-4 sm:px-6 py-6">
      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center py-20">
        <svg class="animate-spin w-6 h-6 text-blue-500" viewBox="0 0 24 24" fill="none">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>
        </svg>
        <span class="ml-2 text-slate-500 text-sm">Loading catalog...</span>
      </div>

      <!-- Empty -->
      <div v-else-if="displayEntries.length === 0" class="text-center py-20">
        <svg class="mx-auto w-12 h-12 text-slate-300 mb-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
          <path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/>
          <polyline points="3.27 6.96 12 12.01 20.73 6.96"/>
          <line x1="12" y1="22.08" x2="12" y2="12"/>
        </svg>
        <h3 class="text-lg font-medium text-slate-600 mb-1">
          {{ search ? 'No matching assets' : 'No assets yet' }}
        </h3>
        <p class="text-sm text-slate-500 mb-4">
          {{ search ? 'Try a different search term.' : 'Add your first asset to get started.' }}
        </p>
        <button
          v-if="!search"
          class="inline-flex items-center gap-1.5 px-4 py-2 text-sm font-medium text-white
                 bg-blue-500 hover:bg-blue-600 rounded-lg cursor-pointer
                 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2
                 transition-colors duration-200"
          @click="openAddModal"
        >
          <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>
          </svg>
          Add Asset
        </button>
      </div>

      <!-- Asset List -->
      <div v-else class="space-y-2">
        <div
          v-for="entry in displayEntries"
          :key="`${entry.kind}-${entry.name}`"
          class="bg-white border border-slate-200 rounded-lg p-4 hover:border-slate-300
                 hover:shadow-sm transition-all duration-200 cursor-default"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2 mb-1">
                <span class="font-medium text-slate-900 text-sm">{{ entry.name }}</span>
                <span
                  class="px-1.5 py-0.5 text-xs font-medium rounded"
                  :class="`kind-badge-${entry.kind}`"
                >
                  {{ entry.kind === 'mcps' ? 'MCP' : entry.kind.slice(0, -1) }}
                </span>
                <span class="px-1.5 py-0.5 text-xs font-medium rounded bg-slate-100 text-slate-500">
                  {{ entry.group || 'Ungrouped' }}
                </span>
              </div>
              <p v-if="entry.description" class="text-sm text-slate-600 mb-1.5 line-clamp-2">
                {{ entry.description }}
              </p>
              <p class="text-xs text-slate-500 font-mono">
                {{ entry.source || '(no source)' }}
              </p>
            </div>

            <div class="flex items-center gap-1 shrink-0">
              <button
                class="p-2 rounded-lg text-slate-400 hover:text-blue-500 hover:bg-blue-50
                       cursor-pointer focus:outline-none focus:ring-2 focus:ring-blue-500
                       transition-colors duration-200"
                aria-label="Edit asset"
                @click="openEditModal(entry)"
              >
                <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
                  <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
                </svg>
              </button>
              <button
                class="p-2 rounded-lg text-slate-400 hover:text-red-500 hover:bg-red-50
                       cursor-pointer focus:outline-none focus:ring-2 focus:ring-red-500
                       transition-colors duration-200"
                aria-label="Delete asset"
                @click="openDeleteConfirm(entry)"
              >
                <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="3 6 5 6 21 6"/>
                  <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
                </svg>
              </button>
            </div>
          </div>
        </div>
      </div>
    </main>

    <!-- Modals -->
    <AddSourceModal
      :visible="addModalVisible"
      @close="addModalVisible = false"
      @added="handleAdded"
    />

    <AssetModal
      ref="editModalRef"
      :visible="editModalVisible"
      :kind="editModalKind"
      :entry="editModalEntry"
      @close="editModalVisible = false"
      @save="handleSave"
    />

    <ConfirmDialog
      :visible="confirmVisible"
      title="Delete Asset"
      :message="`Are you sure you want to delete '${confirmTarget?.name}'? This cannot be undone.`"
      @confirm="handleDelete"
      @cancel="confirmVisible = false"
    />

    <!-- Toasts -->
    <div class="fixed top-4 right-4 z-[60] flex flex-col gap-2">
      <TransitionGroup name="toast">
        <div
          v-for="toast in toasts"
          :key="toast.id"
          class="px-4 py-2.5 rounded-lg shadow-lg text-sm font-medium"
          :class="toast.type === 'success'
            ? 'bg-emerald-500 text-white'
            : 'bg-red-500 text-white'"
        >
          {{ toast.message }}
        </div>
      </TransitionGroup>
    </div>
  </div>
</template>
