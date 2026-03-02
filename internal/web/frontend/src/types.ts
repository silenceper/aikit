export interface CatalogEntry {
  name: string
  source: string
  description: string
  group: string
}

export interface CatalogConfig {
  skills?: CatalogEntry[]
  rules?: CatalogEntry[]
  mcps?: CatalogEntry[]
  commands?: CatalogEntry[]
}

export type AssetKind = 'skills' | 'rules' | 'mcps' | 'commands'

export const KINDS: AssetKind[] = ['skills', 'rules', 'mcps', 'commands']

export const KIND_LABELS: Record<AssetKind, string> = {
  skills: 'Skill',
  rules: 'Rule',
  mcps: 'MCP',
  commands: 'Command',
}

export interface FlatEntry extends CatalogEntry {
  kind: AssetKind
}

export interface DiscoveredItem {
  kind: string
  name: string
  desc: string
}

export interface DiscoverResult {
  source: string
  items: DiscoveredItem[]
}
