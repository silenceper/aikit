# Catalog UI Design Document

> `aikit catalog ui` — A browser-based interface for managing the global asset catalog (`~/.aikit/catalog.yaml`).

## 1. Overview

The Catalog UI provides a local web interface as an alternative to the CLI for managing global catalog assets. It is launched via:

```bash
aikit catalog ui [--host localhost] [--port 9001]
```

The command starts a local HTTP server, opens the default browser, and serves a single-page application (SPA) that communicates with the backend through a REST API. The entire frontend is compiled into the Go binary using `go:embed`, so no external runtime dependencies are required.

### Design Goals

- **Parity with CLI**: The "Add Asset" flow mirrors `aikit catalog add <source>` — enter a source, discover assets, select, and register.
- **Minimal footprint**: Single binary, no external database, no Node.js runtime at serve time.
- **Clean and responsive UI**: Tailwind CSS utility-first styling with a modern, accessible design.

## 2. Architecture

```
┌──────────────────────────────────────────────────────┐
│                    Go Binary (aikit)                  │
│                                                      │
│  cmd/catalog_ui.go          Cobra command entry       │
│         │                                            │
│         ▼                                            │
│  internal/web/server.go     Gin HTTP server           │
│    ├─ GET  /                serve index.html          │
│    ├─ GET  /assets/*        serve static assets       │
│    └─ /api/*                REST API handlers         │
│         │                                            │
│  internal/web/handler.go    API business logic        │
│    ├─ GET    /api/catalog              read catalog   │
│    ├─ POST   /api/catalog/discover     scan source    │
│    ├─ POST   /api/catalog/add-batch    batch register │
│    ├─ POST   /api/catalog/:kind        add one entry  │
│    ├─ PUT    /api/catalog/:kind/:name  update entry   │
│    └─ DELETE /api/catalog/:kind/:name  remove entry   │
│                                                      │
│  internal/web/static/       embedded via go:embed     │
│    ├─ index.html                                     │
│    └─ assets/               JS + CSS bundles          │
│                                                      │
└──────────────────────────────────────────────────────┘
         ▲ built from
┌──────────────────────────────────────────────────────┐
│  internal/web/frontend/     Vue 3 + TypeScript + Vite │
│    ├─ src/App.vue           main layout + state       │
│    ├─ src/api.ts            HTTP client               │
│    ├─ src/types.ts          shared TypeScript types    │
│    ├─ src/style.css         Tailwind + custom styles   │
│    └─ src/components/                                 │
│        ├─ AddSourceModal.vue   discover + select flow │
│        ├─ AssetModal.vue       edit group only        │
│        └─ ConfirmDialog.vue    delete confirmation    │
└──────────────────────────────────────────────────────┘
```

## 3. Tech Stack

| Layer    | Technology                  | Rationale                                           |
| -------- | --------------------------- | --------------------------------------------------- |
| Backend  | Go + Gin                    | Lightweight HTTP framework; already a project dep    |
| Embed    | `go:embed`                  | Bundles frontend into the binary, zero runtime deps  |
| Frontend | Vue 3 + TypeScript          | Reactive SPA with type safety                        |
| Styling  | Tailwind CSS v4 (Vite plugin) | Utility-first, small production CSS bundle          |
| Build    | Vite                        | Fast frontend builds, dev proxy for API              |
| Bundle   | Makefile `frontend` target  | `npm install && npm run build` before `go build`     |

## 4. Backend

### 4.1 CLI Entry Point

`cmd/catalog_ui.go` defines the `catalog ui` Cobra subcommand:

- Flags: `--host` (default `localhost`), `--port` / `-p` (default `9001`)
- Creates an `http.Server` via `web.NewServer(host, port)`
- Starts the server in a goroutine, opens the browser, and blocks until SIGINT/SIGTERM
- Graceful shutdown with a 5-second timeout

### 4.2 Server Setup

`internal/web/server.go`:

- Uses `go:embed static/*` to embed the Vite build output
- Gin in release mode with recovery middleware
- Reads `index.html` at startup via `fs.ReadFile` and serves it with `c.Data()` on `GET /` to avoid `http.FileServer` redirect loops
- Mounts `/assets` as a static filesystem from the embedded FS
- Registers all API routes under the `/api` group

### 4.3 REST API

All endpoints are under `/api` and communicate via JSON.

#### `GET /api/catalog`

Returns the full catalog as JSON. The response shape matches `CatalogConfig`:

```json
{
  "skills":   [{ "name": "...", "source": "...", "description": "...", "group": "..." }],
  "rules":    [...],
  "mcps":     [...],
  "commands": [...]
}
```

#### `POST /api/catalog/discover`

Discovers assets from a source (mirroring `aikit catalog add <source>` interactive mode).

**Request:**

```json
{ "source": "silenceper/ai-assets" }
```

**Behavior:**
1. Resolves the source — for remote repos: normalizes the URL, clones/fetches into `~/.aikit/cache/`; for local paths: resolves to absolute path.
2. Runs `skill.Discover()` to find SKILL.md files and `discovery.DiscoverAll()` to find asset.yaml files.
3. Returns all discovered assets.

**Response:**

```json
{
  "source": "silenceper/ai-assets",
  "items": [
    { "kind": "skill", "name": "code-review", "desc": "Code review best practices" },
    { "kind": "rule",  "name": "respond-in-chinese", "desc": "Always respond in Chinese" },
    { "kind": "mcp",   "name": "playwright", "desc": "Browser automation" }
  ]
}
```

#### `POST /api/catalog/add-batch`

Batch-registers selected assets from the discover step.

**Request:**

```json
{
  "source": "silenceper/ai-assets",
  "group": "Code Quality",
  "items": [
    { "kind": "skill", "name": "code-review", "desc": "Code review best practices" },
    { "kind": "rule",  "name": "respond-in-chinese", "desc": "Always respond in Chinese" }
  ]
}
```

**Behavior:** Iterates through items, delegates to `catalog.AddSkill`, `catalog.AddRule`, `catalog.AddMCP`, or `catalog.AddCommand` based on `kind`. All items share the same `source` and `group`.

**Response:**

```json
{ "message": "added 2 asset(s)" }
```

#### `POST /api/catalog/:kind`

Add a single entry. `:kind` is one of `skills`, `rules`, `mcps`, `commands`.

**Request body:** `CatalogEntry` JSON.

#### `PUT /api/catalog/:kind/:name`

Update an existing entry (used by the edit modal to change the group).

**Request body:** `CatalogEntry` JSON. The `name` path parameter takes precedence.

#### `DELETE /api/catalog/:kind/:name`

Remove an entry by kind and name. Returns 404 if not found.

## 5. Frontend

### 5.1 Project Structure

```
internal/web/frontend/
├── index.html              HTML shell
├── package.json            dependencies (vue, tailwindcss, vite, vue-tsc)
├── vite.config.ts          build output → ../static, dev proxy → localhost:8080
├── tsconfig.json
└── src/
    ├── main.ts             Vue app entry
    ├── App.vue             root component (layout, state, orchestration)
    ├── api.ts              typed HTTP client
    ├── types.ts            shared interfaces
    ├── style.css           Tailwind imports + custom component styles
    └── components/
        ├── AddSourceModal.vue
        ├── AssetModal.vue
        └── ConfirmDialog.vue
```

### 5.2 Type System

```typescript
interface CatalogEntry {
  name: string
  source: string
  description: string
  group: string
}

interface CatalogConfig {
  skills?: CatalogEntry[]
  rules?: CatalogEntry[]
  mcps?: CatalogEntry[]
  commands?: CatalogEntry[]
}

type AssetKind = 'skills' | 'rules' | 'mcps' | 'commands'

interface FlatEntry extends CatalogEntry {
  kind: AssetKind
}

interface DiscoveredItem {
  kind: string   // singular: "skill", "rule", "mcp", "command"
  name: string
  desc: string
}

interface DiscoverResult {
  source: string
  items: DiscoveredItem[]
}
```

### 5.3 Page Layout

The UI is a single-page application with three navigation tiers:

```
┌─────────────────────────────────────────────────┐
│ [Logo] AIKIT Catalog Manager   [Search]  [+Add] │  ← sticky top bar
├─────────────────────────────────────────────────┤
│ All(12)  Skills(5)  Rules(3)  MCPs(2)  Cmds(2) │  ← kind tabs (level 1)
├─────────────────────────────────────────────────┤
│ All Groups(5)  Code Quality(3)  Writing(2)      │  ← group sub-tabs (level 2)
├─────────────────────────────────────────────────┤
│                                                 │
│  ┌─────────────────────────────────────────┐    │
│  │ code-review  [skill] [Code Quality]     │    │  ← asset card
│  │ Code review best practices              │    │
│  │ silenceper/ai-assets        [Edit][Del] │    │
│  └─────────────────────────────────────────┘    │
│                                                 │
│  ┌─────────────────────────────────────────┐    │
│  │ respond-in-chinese  [rule] [Writing]    │    │
│  │ Always respond in Chinese               │    │
│  │ silenceper/ai-assets        [Edit][Del] │    │
│  └─────────────────────────────────────────┘    │
│                                                 │
└─────────────────────────────────────────────────┘
```

**Kind tabs** (level 1): Filter by asset type — All, Skills, Rules, MCPs, Commands. Each tab shows a count badge.

**Group sub-tabs** (level 2): Shown only when there are 2+ groups in the current view. Pill-shaped buttons that filter by group name. "All Groups" shows everything. "Ungrouped" is always sorted last.

**Search**: Full-text filter across name, description, source, and group fields. Applies on top of tab/group filters.

### 5.4 Add Asset Flow (AddSourceModal)

The "Add Asset" button opens a two-step modal that mirrors the CLI `aikit catalog add <source>` workflow:

**Step 1 — Enter Source:**

The user enters a source string:
- GitHub shorthand: `user/repo`
- Git HTTPS URL: `https://github.com/user/repo.git`
- Git SSH URL: `git@github.com:user/repo.git`
- Local path: `.` or `/absolute/path`

Clicking "Discover" calls `POST /api/catalog/discover`. A spinner is shown during the clone/fetch operation.

**Step 2 — Select Assets:**

Discovered assets are shown as a checkbox list (all selected by default):
- Each item shows name, kind badge (color-coded), and description
- "Select all" / deselect all toggle
- "Change source" link to go back to Step 1
- Group name input (defaults to "Ungrouped")

Clicking "Add N Asset(s)" calls `POST /api/catalog/add-batch` and closes the modal on success.

### 5.5 Edit Asset (AssetModal)

The edit modal is restricted to modifying the **group** field only. All other fields (type, name, source, description) are displayed as read-only text. This mirrors the fact that these fields are derived from the source repository and should not be manually altered.

### 5.6 Delete Asset (ConfirmDialog)

A confirmation dialog with the asset name. On confirm, calls `DELETE /api/catalog/:kind/:name`.

### 5.7 Toast Notifications

Success/error toasts appear in the top-right corner with auto-dismiss after 3 seconds. Slide-in/out animation on the X axis.

## 6. Build Pipeline

The frontend is built as part of the standard `make build` process:

```
make build
  └─ make frontend
  │    └─ cd internal/web/frontend && npm install && npm run build
  │         └─ vue-tsc -b && vite build
  │              └─ output → internal/web/static/
  │                   ├─ index.html
  │                   └─ assets/
  │                        ├─ index-*.css  (~24 KB, ~5 KB gzip)
  │                        └─ index-*.js   (~95 KB, ~35 KB gzip)
  └─ go build -o bin/aikit .
       └─ go:embed internal/web/static/* → binary
```

During development, `npm run dev` starts Vite's dev server with hot-reload and proxies `/api` requests to the Go backend (running separately).

`make clean` removes both `bin/` and `internal/web/static/`.

## 7. Data Flow

### Add Flow

```
User enters source → [Discover] → POST /api/catalog/discover
                                        │
                       Backend: resolveSource() → git clone/fetch
                                  skill.Discover() + discovery.DiscoverAll()
                                        │
                                        ▼
                              Return discovered items
                                        │
User selects items + group → [Add] → POST /api/catalog/add-batch
                                        │
                       Backend: catalog.AddSkill/AddRule/AddMCP/AddCommand
                                  writes ~/.aikit/catalog.yaml
                                        │
                              Frontend: reload catalog, show toast
```

### Edit Flow

```
User clicks Edit → modal opens (group field editable)
User changes group → [Save] → PUT /api/catalog/:kind/:name
                                   │
                      Backend: catalog.Add*() (upsert by name)
                                   │
                             Frontend: reload catalog
```

### Delete Flow

```
User clicks Delete → confirm dialog
User confirms → DELETE /api/catalog/:kind/:name
                     │
        Backend: catalog.Remove*()
                     │
               Frontend: reload catalog
```

## 8. Styling

The UI uses **Tailwind CSS v4** via the Vite plugin (`@tailwindcss/vite`).

### Theme

- Font: Inter (sans), SF Mono / Fira Code (mono)
- Background: `slate-50`, Cards: `white` with `slate-200` borders
- Primary: `blue-500` / `blue-600`
- Destructive: `red-500`
- Success toast: `emerald-500`

### Kind Badge Colors

| Kind     | Background   | Text          |
| -------- | ------------ | ------------- |
| Skills   | `blue-50`    | `blue-600`    |
| Rules    | `green-50`   | `green-600`   |
| MCPs     | `yellow-50`  | `yellow-600`  |
| Commands | `purple-50`  | `purple-600`  |

### Animations

- Modal: fade + translateY(-8px) over 250ms
- Toast: fade + translateX(16px) over 200ms
- Respects `prefers-reduced-motion`

## 9. File Inventory

### Backend

| File                          | Purpose                                          |
| ----------------------------- | ------------------------------------------------ |
| `cmd/catalog_ui.go`           | Cobra command, server lifecycle, browser open     |
| `internal/web/server.go`      | Gin server setup, embed FS, route registration    |
| `internal/web/handler.go`     | API handlers: CRUD, discover, batch-add           |

### Frontend

| File                                       | Purpose                                     |
| ------------------------------------------ | ------------------------------------------- |
| `internal/web/frontend/package.json`       | npm dependencies and scripts                 |
| `internal/web/frontend/vite.config.ts`     | Vite build config, output dir, dev proxy     |
| `internal/web/frontend/index.html`         | HTML entry point                             |
| `internal/web/frontend/src/main.ts`        | Vue app bootstrap                            |
| `internal/web/frontend/src/App.vue`        | Root component: layout, tabs, state, search  |
| `internal/web/frontend/src/api.ts`         | Typed API client (fetch wrapper)             |
| `internal/web/frontend/src/types.ts`       | TypeScript interfaces and constants          |
| `internal/web/frontend/src/style.css`      | Tailwind imports + custom component classes  |
| `internal/web/frontend/src/components/AddSourceModal.vue`  | Two-step add flow       |
| `internal/web/frontend/src/components/AssetModal.vue`      | Edit group modal        |
| `internal/web/frontend/src/components/ConfirmDialog.vue`   | Delete confirmation     |

### Build Artifacts (gitignored)

| Path                          | Content                                |
| ----------------------------- | -------------------------------------- |
| `internal/web/static/`        | Vite build output, embedded into binary |
| `internal/web/frontend/node_modules/` | npm dependencies               |
