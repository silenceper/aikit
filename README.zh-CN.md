[English](README.md) · **简体中文**

# aikit

[![CI](https://github.com/silenceper/aikit/actions/workflows/ci.yml/badge.svg)](https://github.com/silenceper/aikit/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/silenceper/aikit?include_prereleases)](https://github.com/silenceper/aikit/releases)
[![License](https://img.shields.io/github/license/silenceper/aikit)](LICENSE)

**aikit 是一个面向 AI 编程 Agent 的本地优先 Skills 管理器。** 它使用一份
权威配置账本和一个中央 Skill Library，再把受管链接同步到 Cursor、Claude
Code、Codex、GitHub Copilot 与 Windsurf 的全局或项目工作区。

日常操作可以使用支持鼠标的全屏 TUI，脚本和 CI 则可以使用确定性的 CLI。

## 为什么选择 aikit

Agent Skills 很容易散落在不同 IDE 的目录里，逐渐形成多份互不一致的副本，
最终连这些基本问题也很难回答：

- 哪一份才是权威内容？
- 一个 Skill 在哪些位置启用了？
- 项目里的副本是否已经偏离 Library？
- 更新或清理被中断后，能否继续安全恢复？

aikit 为这些问题明确了唯一归属：

- `$AIKIT_HOME/config.yaml` 是持久化配置账本。
- `$AIKIT_HOME/library/skills/<id>` 是中央内容 Library。
- Agent 目录保存受管链接，而不是各自维护独立副本。
- 全局、项目与 Preset 绑定引用稳定的 Skill ID。
- Pending Operation 与 Library Journal 保存显式恢复状态。

## 项目状态

> [!IMPORTANT]
> aikit 目前仍是 Alpha 软件。它采用面向生产安全的设计，但 CLI、TUI、配置与
> 恢复元数据在 v1 之前仍可能变化。请为重要配置保留可恢复备份，并在收归
> 现有目录之前先检查 dry-run 结果。

本文档描述当前 `main` 分支。最新已发布 Alpha 早于目前的 TUI、恢复机制与
完整工作流；如需使用本文描述的能力，请从源码构建。Release 压缩包仍可用于
评估较早的已发布 Alpha。

当前版本仅管理 **Skills**。Rules、MCP 配置、命令包、Web UI、导入导出、
跨设备同步以及企业级支持目前都不在范围内。

macOS 和 Linux 是当前主要运行目标。Release 配置会为 Linux、macOS、Windows
的 amd64 与 arm64 构建产物，CI 也会交叉编译这些目标；但交叉编译并不等于
原生行为认证。尤其是依赖 Unix 锚定目录操作的受认证链接恢复在 Windows 及
其他不支持的平台上不可用，相关操作会 fail closed，而不是冒险继续写入。

版本历史与变化请查看 [CHANGELOG](CHANGELOG.md)。

## 核心能力

- **单一中央 Library** — 发现、哈希、添加、查看、更新、比较和删除 Skill，
  不再把 Agent 目录当作多个独立来源。
- **本地与 Git 来源** — 支持本地目录、Git URL、GitHub `owner/repo` 简写，
  以及精确的 `skills.sh/<owner>/<repo>/<skill>` 地址。
- **全局与项目 Scope** — 可全局启用，也可绑定到项目 Common Scope 或指定
  项目 Agent。
- **可复用 Preset** — 维护具名 Skill 集合，并应用到精确的全局或项目 Scope。
- **交互式 TUI** — 键盘与鼠标操作、响应式布局、预览、精确确认、过滤、
  多选动作以及清晰的执行状态。
- **离线本地盘点** — 启动时扫描全部受支持的全局目录和已登记项目，不拉取
  Git，也不修改账本。
- **更新与固定 Ref** — 缓存 Branch 更新检查；Tag 与 Commit 保持固定；保留
  原始 Git Transport。
- **显式迁移与恢复** — 在确认前预览 Import/Adopt，并在新写操作前审查待恢复
  事务。
- **Fail-closed 清理** — 无法证明受管归属时保留未知或已被替换的内容。

## 界面预览

<!-- TUI 视觉布局稳定后再补充真实截图。 -->

## 支持的 Agent 与平台

| Agent ID | 全局 Skill 目录 | 项目 Skill 目录 |
|---|---|---|
| `cursor` | `~/.cursor/skills` | `<project>/.cursor/skills` |
| `claude-code` | `~/.claude/skills` | `<project>/.claude/skills` |
| `codex` | `~/.codex/skills` | `<project>/.codex/skills` |
| `copilot` | `~/.copilot/skills` | `<project>/.agents/skills` |
| `windsurf` | `~/.codeium/windsurf/skills` | `<project>/.windsurf/skills` |

| 平台 | Release 构建 | 当前支持边界 |
|---|---|---|
| macOS amd64/arm64 | 是 | 主要运行目标 |
| Linux amd64/arm64 | 是 | 主要运行目标 |
| Windows amd64/arm64 | 是 | 可构建及基础流程；锚定链接恢复会 fail closed |

## 安装

### 从源码构建（推荐）

需要 Go 1.25 或更高版本。这是当前获得本文所述完整功能的受支持方式。

```bash
git clone https://github.com/silenceper/aikit.git
cd aikit
make build
./bin/aikit version
```

### 已发布 Alpha 压缩包

从 [GitHub Releases](https://github.com/silenceper/aikit/releases) 下载与你的
操作系统和架构匹配的压缩包，并使用同一 Release 中的 `checksums.txt` 校验后，
再把 `aikit` 放入 `PATH`。这些压缩包目前是较早的已发布 Alpha，尚未包含
`main` 文档中的全部工作流。

Linux 与 macOS 使用 `.tar.gz`，Windows 使用 `.zip`。

Homebrew 发布链路完成端到端验证前，本文档暂不提供其安装命令。

## 快速开始

### 1. 添加 Skill

添加本地 Skill，并为 Codex 全局启用：

```bash
aikit add ./my-skill --agent codex
```

从 Git 仓库添加指定 Skill：

```bash
aikit add vercel-labs/agent-skills --skill code-review
aikit add https://skills.sh/vercel-labs/agent-skills/find-skills
aikit add https://gitlab.example.com/team/skills.git \
  --skill review --skill release
```

精确的 `skills.sh` 页面会建议选择对应 Skill；显式、可重复的 `--skill` 总是
覆盖该建议。GitHub `owner/repo` 简写会解析到 GitHub。aikit 会直接克隆底层
仓库，不执行 `npx`，也不依赖 Marketplace API。

### 2. 登记项目

```bash
aikit project add /path/to/project \
  --name my-project \
  --agent cursor \
  --agent codex
```

TUI 提供 Path-first 流程：先输入项目路径，再自动推导名称、检测 Agent 目录，
最后展示精确预览。

### 3. 为项目启用 Skill

```bash
aikit enable vercel-labs/agent-skills/code-review \
  --project my-project \
  --agent codex
```

同时指定 `--project` 但省略 `--agent` 时，目标是项目 Common Scope。

### 4. 创建并应用 Preset

```bash
aikit preset create essentials \
  --skill vercel-labs/agent-skills/code-review
aikit enable --preset essentials --agent cursor
```

### 5. 查看并对账

```bash
aikit status --offline
aikit sync --dry-run
aikit sync
```

## TUI

在终端直接运行不带参数的 `aikit`。非 TTY 环境必须使用明确的 CLI 命令，
不会意外进入交互界面。

导航区提供这些日常入口：

- **Overview** — 待处理事项、更新摘要和常用操作入口。
- **Library** — 受管 Skills、详情、来源添加、Ref、更新与多选操作。
- **Presets** — 可复用集合、成员维护、复制、重命名、删除和精确 Scope 应用。
- **Global / Agents / Projects** — 直接进入工作区绑定与项目管理。
- **Configuration** — 展示实际 Config、Library、Cache 路径，并提供只读校验与
  Reload。

Health 详情、本地导入审查和显式恢复会在需要时通过命令面板提供。

常用操作：

| 按键 | 操作 |
|---|---|
| 鼠标点击 / 滚轮 | 选择、触发或滚动当前可见控件 |
| `j` / `k`、方向键 | 在当前焦点区域移动 |
| `Tab` / `Shift+Tab` | 在导航、列表、详情与操作区之间切换焦点 |
| `Enter` | 打开或触发当前焦点项 |
| `Space` | 多选，或在支持的位置切换绑定 |
| `/` | 过滤当前集合 |
| `Ctrl+P` 或 `:` | 打开命令面板 |
| `Ctrl+K` | 直接打开 Configuration |
| `Esc` | 返回或取消；在主界面不会直接退出 |
| `Ctrl+Q` | 正常退出 |
| `Ctrl+C` | 紧急退出，包括操作执行期间 |
| `?` | 打开上下文帮助 |

启动时会先渲染本地账本，再增量盘点所有受支持的全局 Agent 目录与已登记项目。
启动盘点是离线、只读的：不会拉取 Git、导入内容、收归路径、重写配置或修改
中央 Library。

写操作会先构建 Typed Preview，并要求显式确认。远程 **Add source** 的第一次
确认只允许临时 Git 发现；第二个界面用于精确选择候选，确认后才会添加。

## CLI

CLI 适合确定性自动化，并支持全局 `--json` 输出。

```text
aikit add <source-or-path> [--skill ...] [--agent ...] [--project ...]
aikit list [--agent ...] [--project ...] [--preset ...] [--offline]
aikit remove <id> [--force] [--yes]
aikit enable [id] | --preset <name> --agent/--project
aikit disable [id] | --preset <name> --agent/--project
aikit preset create|add|remove|list
aikit project add|edit|remove|list
aikit sync [--agent ...] [--project ...] [--dry-run]
aikit status [--offline] [--refresh]
aikit update [id] [--skill ...] [--check] [--yes]
             [--ref branch:...|tag:...|commit:...]
aikit scan [--agent ...] [--project ...] [--skill ...] [--all] [--adopt]
aikit migrate [--project ...] [--dry-run] [--adopt]
```

完整参数以 `aikit <command> --help` 为准。

目前有些高级流程只在 TUI 中提供独立入口，包括 Configuration 校验/Reload、
Library 原子批量操作、Preset 重命名/复制、Skill Compare，以及显式 Recovery
Review/Resume。CLI 仍可创建、增删成员、列出、删除和应用 Preset。

### 更新与 Ref

远程 Skill 会记录 Canonical Source、仓库内 Source Path、结构化 Ref 与完整的
Resolved Object ID。Branch Ref 会参与更新检查；Tag 与 Commit Ref 保持固定。

```bash
aikit update --check
aikit update --yes
aikit update <id> --ref tag:v2.0.0 --force --yes
aikit status --offline
```

非 TTY 环境中，没有 `--yes` 的 Update 只执行检查。

### 退出码

| 退出码 | 含义 |
|---|---|
| `0` | 操作完成且没有报告问题 |
| `1` | 错误或部分结果，包括 Drift、Conflict 或 Pending Recovery |
| `2` | 存在可用更新，但没有修改内容 |

## 配置与数据

`AIKIT_HOME` 默认是 `~/.aikit`，最终会解析为本机绝对路径。

| 路径 | 用途 |
|---|---|
| `$AIKIT_HOME/config.yaml` | 持久化全局账本 |
| `$AIKIT_HOME/config.lock` | 跨进程写操作锁 |
| `$AIKIT_HOME/library/skills/<id>` | 中央 Skill 内容 |
| `$AIKIT_HOME/cache/` | Git Mirror、临时来源数据与更新缓存 |

测试或隔离 Profile 可以指定独立 Home：

```bash
AIKIT_HOME=/absolute/path/to/aikit-home aikit status --offline
```

aikit 运行期间不要手工修改账本。手工修复前先备份，保留 Pending Operation
字段，并通过 TUI Recovery Review 处理，而不是直接删除 Journal 或 Quarantine。

## 安全与恢复

aikit 的文件系统模型以 fail closed 为原则：

- 所有写操作由 `$AIKIT_HOME/config.lock` 串行化；
- 配置 Checkpoint 使用原子替换；
- Library 变更使用 Staging、Journal 与 Ledger-directed Mutation；
- 正常 Reconcile 不覆盖真实目录或未知文件；
- 破坏性清理会认证预期受管对象，并保留已替换或无法识别的内容；
- 来源发现与内容读取使用 Containment 和 No-follow 校验；
- 错误会脱敏 Authorization Header、嵌入式凭据与敏感 URL；
- Preview、Dry-run、离线 Status 与启动盘点保持只读；
- Pending Recovery 在显式审查前会阻止无关写操作。

任何本地文件系统工具都无法防御所有特权进程或恶意同用户进程。请为重要工作
保留操作系统级备份；遇到异常 Conflict 时先检查，不要手工删除 `.aikit-*`
Artifact。

## 故障排查

### 提示必须指定命令

stdin 不是 TTY，因此 aikit 正确拒绝打开全屏界面。请改用明确命令，例如
`aikit list --offline` 或 `aikit status --offline`。

### 路径被占用或清理被阻止

先运行：

```bash
aikit status --offline
aikit sync --dry-run
```

aikit 会保留未知内容，而不是直接覆盖。确认归属后，再手工移动或处理报告路径。

### Pending Recovery 阻止新的修改

打开 TUI，从命令面板或 Attention Queue 选择 **Review recovery**。继续前仔细
核对 Operation ID 与路径。不要直接删除账本中的 Pending 项或 `.aikit-*`
Artifact。

### 远程更新检查失败

使用 `aikit status --offline` 查看纯本地状态。确认相同 Git URL 能通过日常 Git
Credential Helper 或 SSH Agent 使用。aikit 拒绝带嵌入式 Userinfo 的 HTTP(S)
来源；不要把 Token 放进 Source URL。

### 使用了意外的配置文件

检查 `AIKIT_HOME`，然后在 TUI 按 `Ctrl+K` 查看最终解析的 Config、Library 与
Cache 路径。

## 贡献与支持

- 提交 PR 前请阅读 [CONTRIBUTING.md](CONTRIBUTING.md)。
- 通过 [SUPPORT.md](SUPPORT.md) 选择正确的问题反馈渠道。
- 按照 [SECURITY.md](SECURITY.md) 私密报告安全问题。
- 所有社区参与都应遵守 [行为准则](CODE_OF_CONDUCT.md)。
- 面向用户的变化记录在 [CHANGELOG.md](CHANGELOG.md)。

开发验证：

```bash
make check
make test-e2e
```

E2E 只使用临时本地目录和本地 Skill，不需要网络，也不会修改真实 Agent 目录。

## 许可证

aikit 使用 [Apache License 2.0](LICENSE)。
