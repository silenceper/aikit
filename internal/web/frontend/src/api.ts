import type { CatalogConfig, CatalogEntry, AssetKind, DiscoverResult, DiscoveredItem } from './types'

const BASE = '/api'

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(BASE + url, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
  return data as T
}

export function getCatalog(): Promise<CatalogConfig> {
  return request('/catalog')
}

export function addEntry(kind: AssetKind, entry: CatalogEntry): Promise<{ message: string }> {
  return request(`/catalog/${kind}`, {
    method: 'POST',
    body: JSON.stringify(entry),
  })
}

export function updateEntry(kind: AssetKind, name: string, entry: CatalogEntry): Promise<{ message: string }> {
  return request(`/catalog/${kind}/${encodeURIComponent(name)}`, {
    method: 'PUT',
    body: JSON.stringify(entry),
  })
}

export function deleteEntry(kind: AssetKind, name: string): Promise<{ message: string }> {
  return request(`/catalog/${kind}/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  })
}

export function discoverAssets(source: string): Promise<DiscoverResult> {
  return request('/catalog/discover', {
    method: 'POST',
    body: JSON.stringify({ source }),
  })
}

export function batchAddAssets(source: string, group: string, items: DiscoveredItem[]): Promise<{ message: string }> {
  return request('/catalog/add-batch', {
    method: 'POST',
    body: JSON.stringify({ source, group, items }),
  })
}
