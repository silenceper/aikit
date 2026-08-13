# aikit 全局 Skills 管理（CLI + TUI）

日期：2026-08-13
状态：已修订（实现前待用户复核本文）

第一版只做 **Skills**。以本机一份全局 YAML 为唯一账本，中央库存文件，IDE 目录只放 symlink。日常用 TUI，脚本用 CLI。Rules / MCP / Commands / Web 不在本范围。

现有 `.aikit.yaml` 项目账本、`catalog.yaml` 收藏夹语义本设计不沿用。用户磁盘上的旧数据通过显式 `aikit migrate` 转入，不静默覆盖。

**现有代码可以整段废弃。** 实现按本文从零搭 CLI/TUI/对账引擎，不要求兼容旧子命令或旧包结构。旧代码里若有可抄的片段（例如 git clone、路径拼接）可以顺手拿，没有义务保留。

## 1. 目标与非目标

### 目标

- 全局配置记录：技能库、presets、各 IDE 全局工作区、各项目工作区。
- 覆盖 IDE 的 skills 目录（用户级 + 项目级），方式为 symlink overlay。
- 同名 skill 可在库中并存（用 `source/name` 作 id）；同一 IDE 目标目录短名冲突则拒绝。
- 存量可导入库；只有明确 adopt 才把现场换成 symlink。
- Git source 与任意本地路径都可入库；入库本身不改变任何 IDE 目录。
- 无参数进入 **k9s 风格全屏 TUI**（列表点选、键盘操作）；参数齐全的命令直接执行。
- 远端 skill 有更新时**提示并让用户选择是否同步**，不静默覆盖。

### 非目标（第一版）

- Web UI、多机器面板
- Rules / MCP / Commands
- 项目内再写一份账本 YAML（账本只有 `~/.aikit/config.yaml`）
- 自动改短名（如 `code-review-2`）来消解 IDE 目录冲突
- 在非 TTY / CI 中弹出 TUI
- 远端更新自动写入（必须确认或 `--yes`）
- 跨机器复制、export / import、多机器同步

## 2. 目录与数据模型

家目录为 `~/.aikit`，若设置了 `AIKIT_HOME` 则用该路径替换下文所有 `~/.aikit`。

```
$AIKIT_HOME 或 ~/.aikit/
├── config.yaml                 # 唯一账本
├── library/skills/<id>/        # 技能正文（SKILL.md 等）
└── cache/                      # 远程 git 克隆，供 add / update
```

`<id>` 为路径安全形式，例如 `vercel-labs/agent-skills/code-review`，磁盘上即：

```
~/.aikit/library/skills/vercel-labs/agent-skills/code-review/SKILL.md
```

四层数据互不继承：

| 层 | 作用 | 是否进入 IDE |
|----|------|----------------|
| Library | 收藏 | 否 |
| Presets | 一组 skill id | 否（被引用后才落地） |
| Agents | 某 IDE 用户级启用 | `~/.<agent>/skills/<短名>` |
| Projects | 某项目启用 | `<project>/.<agent>/skills/<短名>` |

全局和项目是两套绑定。IDE 会同时读用户级和项目级目录，因此「所有仓库都要」放全局，「这个仓库才要」放项目。

### 2.1 Skill 身份

- **id**（库内唯一）：`<normalized-source>/<name>`
  - GitHub shorthand：`vercel-labs/agent-skills/code-review`
  - 其它 git URL：先规范化为稳定 source（与现有 cache 目录规则一致），再拼 `name`
  - 无远程源的本地导入：按下面「本地 id」规则，**不要**用 `local/<agent>/<name>`
- **name**（短名）：来自 `SKILL.md` frontmatter / 目录名，用作 IDE 下的文件夹名
- CLI/TUI：短名在库中唯一时可省略 id；一旦撞名必须写全 id

**hash**：对 skill 目录内所有文件做内容哈希（相对路径排序后拼接），忽略 `.git/`。不比较时间戳。hash 相同视为同一份内容。

**本地 id**（`scan` / 从无 git 源的本地路径 `add`）：

1. 若库中已有同一短名且同一 hash 的条目（无论其 id），复用该 id，不建第二份。
2. 否则若 `local/<name>` 尚未占用：id = `local/<name>`
3. 否则：id = `local/<name>-<hash 前 12 位 hex>`

同一次 scan 里同一短名出现多种 hash、且库中都还没有：按来源稳定排序后，第一个走规则 2，其余走规则 3。来源排序键：全局为 `g/<agent>`（agent 按 §2.3 表顺序）；项目为 `p/<projectName>/<agent>`（项目名字符串序，同一项目内再按 §2.3 Agent 表顺序）。

同一 Agent 的「全局 vs 项目、内容不同」因此得到两个稳定 id，例如 `local/code-review` 与 `local/code-review-a1b2c3d4e5f6`。

preset / agent / project 的引用一律存 **id**，不存短名。

同一 Agent 或同一「项目 × Agent」目标里，展开后的短名不得重复。冲突时该目标不改盘，报错并列出两个 id。

### 2.2 `config.yaml`

```yaml
library:
  skills:
    - id: vercel-labs/agent-skills/code-review
      name: code-review
      source: vercel-labs/agent-skills
      ref: v1.2.0                    # 可选：tag / commit / branch
      resolved: a1b2c3d              # 入库/上次 update 时的 commit 短 SHA，供检测更新
      description: "..."

presets:
  - name: essential-for-dev
    skills:
      - vercel-labs/agent-skills/code-review

agents:
  cursor:
    presets: [essential-for-dev]
    skills:
      - vercel-labs/agent-skills/web-design-guidelines
  claude-code:
    presets: [essential-for-dev]

projects:
  - name: aikit
    path: /Users/you/workspace/go/src/github.com/silenceper/aikit
    agents: [cursor, claude-code]
    presets: [essential-for-dev]
    skills: []
    agent_bindings:
      cursor:
        presets: []
        skills:
          - vercel-labs/agent-skills/web-design-guidelines
```

Agent 全局有效集合 = `agents.<agent>.presets` 展开 ∪ `agents.<agent>.skills`。

项目公共集合 = `projects[].presets` 展开 ∪ `projects[].skills`，应用到该项目 `agents` 中的每个 Agent。项目中特定 Agent 的目标集合 = 项目公共集合 ∪ `projects[].agent_bindings.<agent>.presets` 展开 ∪ `projects[].agent_bindings.<agent>.skills`。`agent_bindings` 的 key 必须同时出现在该项目的 `agents` 中。

所有层级都只保存 preset **名**（不展开）；修改 preset 后，所有引用该 preset 的全局与项目目标在下次 `sync` 时一起更新。一个 skill 同时被直接引用和 preset 引用时按集合去重。

`projects[].path` 为本机绝对路径。`projects[].name` 在配置内唯一，用于 CLI/TUI 与 `--project`；规范化后的真实 path 也必须唯一，禁止同一路径登记成两个项目。`presets[].name` 同样唯一。

### 2.3 Agent 与落地路径

第一版五个 Agent：

| Agent 名 | 全局 skills | 项目 skills |
|----------|-------------|-------------|
| `cursor` | `~/.cursor/skills/<name>` | `<project>/.cursor/skills/<name>` |
| `claude-code` | `~/.claude/skills/<name>` | `<project>/.claude/skills/<name>` |
| `codex` | `~/.codex/skills/<name>` | `<project>/.codex/skills/<name>` |
| `copilot` | `~/.copilot/skills/<name>` | `<project>/.agents/skills/<name>` |
| `windsurf` | `~/.codeium/windsurf/skills/<name>` | `<project>/.windsurf/skills/<name>` |

目标目录不存在则创建。不要求事先 Detect 到 IDE 配置。

symlink 目标一律指向 `$AIKIT_HOME/library/skills/<id>`（解析后的绝对路径）。

账本与 CLI 只使用上表「Agent 名」。旧适配器名 `github-copilot` 在 migrate / 任何读入处映射为 `copilot`，不得把旧名写入 `config.yaml`。

## 3. 对账引擎

`enable` / `disable` / `sync` / `project remove` 走 **普通对账**。`scan --adopt` 走 **adopt 换链**（§5.1），不套用下表「真目录不覆盖」。

「由 aikit 管理」= 该路径是 symlink，且最终目标在 `$AIKIT_HOME/library/skills/` 之下。

### 3.1 普通对账

| 磁盘现状 | YAML 期望有 | YAML 期望无 |
|----------|-------------|-------------|
| 无此路径 | 建 symlink | 不动 |
| 正确指向该 id 的 symlink | 跳过 | 断链 |
| symlink 指向库内另一 id | 改指向 | 断链 |
| 断掉的 symlink | 删掉再建 | 删掉 |
| 真目录 / 真文件 | 不覆盖，记冲突，其它路径继续 | 不删 |
| 指向库外的 symlink | 不覆盖，记冲突 | 不删 |

写盘顺序：**先原子写 YAML**（temp + rename），再对账磁盘。写盘失败时账本已是新状态，`status` 显示 drift，再 `sync` 修复。禁止「目录已变、账本未记」。

`sync --dry-run` 只打印将建 / 将拆 / 冲突。

### 3.2 全局与项目的有效视图

对一个「项目 × Agent」，IDE 最终看到的集合由该 Agent 的全局集合与该项目 Agent 目标集合共同决定。aikit 必须按短名构造有效视图：

- 两层没有同名：正常落地。
- 两层同名且 id 相同：视为同一个 skill。保留两层 YAML 绑定，但只创建全局 symlink；项目视图显示为 `inherited + project`。以后若关闭全局绑定，普通对账自动在项目目录补建 symlink。
- 两层同名但 id 不同：这是跨层冲突。引入冲突的 `enable`、preset 修改或项目 Agent 修改必须在写 YAML 前拒绝，并列出 Agent、项目和两个完整 id。

同一层内部的短名冲突仍按 §2.1 拒绝。若用户手改 YAML 制造冲突，`status` 报 `scope-conflict`；`sync` 跳过该「项目 × Agent」，其它目标继续，退出码非 0。

所有会扩大有效集合的操作（enable、preset 成员变更、project edit、scan --adopt、migrate）都必须预先校验其全部受影响目标，不能写入一个已知冲突的账本。

### 3.3 sync 范围

`--agent` 与 `--project` 可单独或同时出现：

| 调用 | 对账哪些目录 |
|------|----------------|
| `aikit sync` | 全部 Agent 的**全局**目录 + 全部已登记且 path 存在的项目的全部已声明 Agent 目录 |
| `aikit sync --agent cursor` | **仅** Cursor 全局目录（`~/.cursor/skills`），不含任何项目 |
| `aikit sync --project aikit` | 仅该项目 `agents` 列表里的项目级目录 |
| `aikit sync --agent cursor --project aikit` | 仅该项目的 Cursor 项目级目录 |

项目 `path` 不存在：警告并跳过该项目，不中止其它目标。

短名冲突：仅跳过该目标（一个全局 Agent 目录，或一个「项目 × Agent」目录），其它目标继续；进程退出码非 0。

### 3.4 enable / disable 的 `--agent` / `--project`

- 只 `--agent X`：改 `agents.X` 绑定，立刻普通对账 X 的全局目录
- 只 `--project P`：改该项目的公共 `skills`/`presets`，立刻普通对账该项目全部已声明 Agent 的项目级目录
- `--project P --agent X`：只改 `projects[P].agent_bindings.X`，即只在该项目的 X 中启用；X 不在项目 `agents` 中则拒绝，并提示先 `project edit --add-agent`

`disable` 与上面完全对称。若需要同时修改全局和项目，用户分别执行两条命令；一个命令不隐式修改两个作用域。

## 4. CLI

非 TTY 且缺少必填参数：报错退出，不进 TUI。

仅 `enable` / `disable` / `scan` 在已登记项目目录内可省略 `--project`。cwd 经 `EvalSymlinks` 后等于项目 path 或位于其下都算命中；嵌套项目取最长祖先 path。`project add` 不需要推断，未给 path 时直接使用 cwd。`sync` 与 `status` 不因 cwd 缩小范围，始终以 §3.3 与全量状态为准。

| 命令 | 行为 |
|------|------|
| `aikit` | TTY 下进入主菜单 TUI |
| `aikit add <source-or-path>` | Git source 或本地路径中发现并入库；多项且无 `--skill` 时交互勾选 |
| `aikit add <source-or-path> --skill <id\|短名>` | 入库指定 skill；本地路径按发现短名选择 |
| `aikit add ... --agent/--project` | 入库并按 §3.4 立刻 enable；同时给 project+agent 表示项目内指定 Agent |
| `aikit scan` | 扫描本机 IDE 目录，拷进库，不改现场 |
| `aikit scan --adopt` | 见 §5.1；TTY 勾选，非 TTY 必须 `--skill` 或 `--all` |
| `aikit remove <id>` | 出库；仍被引用则拒绝 |
| `aikit remove <id> --force` | 先从所有 preset/agent/project 解绑并断链，再出库 |
| `aikit list` / `--agent` / `--project` / `--preset` | 只读列表 |
| `aikit update [id]` | 先探测；TTY 列出可更新项并确认后才写库。无 id：所有未 pin 的远程条目。`--check` 只探测。`--yes` 跳过确认。规则见 §4.1 |
| `aikit preset create <name> --skill <id>...` | 建 preset |
| `aikit preset add <name> --skill <id>` | 向 preset 加成员 |
| `aikit preset remove <name> [--skill <id>]` | 无 `--skill` 删除整个 preset（仍被 agent/project 引用则拒绝，`--force` 同时从引用处去掉该 preset 名） |
| `aikit preset list` | 列表 |
| `aikit enable <id> --agent/--project` | 见 §3.4 |
| `aikit enable --preset <name> --agent/--project` | 同上 |
| `aikit disable ...` | 对称 |
| `aikit project add [path]` | 默认 `.`；询问/参数指定 agents；name 默认目录名，重名拒绝 |
| `aikit project list` | |
| `aikit project edit <name>` | `--name` / `--path` / `--add-agent` / `--remove-agent`；见 §4.2 |
| `aikit project remove <name>` | 先从 YAML 删除该项目，再按「期望无」普通对账拆除其 aikit symlink。不删用户真目录 |
| `aikit status` | 只读：库数量、绑定、drift、未纳管项、可更新数；不会询问或执行 update |
| `aikit update --check` | 只探测，不写库；打印落后条目 |
| `aikit sync` | 按 YAML 全量对账（只对账 symlink，**不**拉远端） |
| `aikit migrate` | 见 §5.2 |

`add` 时 id 已存在：拒绝；`--force` 覆盖库文件，保留已有绑定。

`enable` 的 id 不在 library、或 preset 引用缺失 skill：拒绝，不改盘。

`aikit update` 与 `ref`：

- `ref` 为空，或为 branch：`git fetch` 后更新到该 branch（空则远程默认分支）的当前提交，并把 cache 中该 skill 目录重拷到 library，写入新的 `resolved`
- `ref` 为 tag 或 commit：默认不前进，打印 pinned 并跳过；`--force` 仍检出**同一个** pin 再重拷（用于修复库文件损坏，不会升到更新的 tag）
- 无远程 `source` 的本地导入（id 以 `local/` 开头）：`update` 跳过
- 改 pin：编辑 YAML 里的 `ref` 后执行 `aikit update --force [id]`
- `disable <id>`：只从该目标的 `skills[]` 去掉该 id。若有效集合仍包含它（来自 preset），磁盘不变并打印提示；若用户就是要从 preset 里拿掉，应改 preset，而不是 disable

本地路径 `add`：

- 参数在本机存在时按 path 处理，否则按 Git source 处理；不存在且也不是合法 Git source 时报错
- path 根目录有 `SKILL.md` 时视为单个 skill；否则递归发现其下所有 `SKILL.md`
- 内容复制到 library 后不再依赖原 path；library 条目省略远程 `source/ref/resolved`，记录内容 `hash`
- `add` 只入库；只有显式给 `--agent` / `--project` 才继续 enable

### 4.1 远端更新探测与确认

「有更新」= 该 skill 有 git `source`，且 `ref` 为空或为 branch，`git fetch` 后远程该 branch 的 HEAD ≠ `resolved`。pinned tag/commit 不参与「有更新」提示（除非用户 `update --force` 修文件）。

**不自动写入。** 探测到更新后必须确认：

| 环境 | 行为 |
|------|------|
| TUI | 顶栏显示 `↑ 3 updates`；`u` 打开可更新列表，空格勾选，`enter` 确认后才 `update` 选中项 |
| TTY 的 `aikit update` 发现更新 | 列出 id 与新旧 SHA，问是否同步（可多选）；否或 Ctrl-C 则不写库 |
| TTY 的 `aikit status` 发现更新 | 只打印可更新项和建议命令 `aikit update`，不询问、不写库 |
| 非 TTY | `update` 只探测并打印，退出码 0 表示无更新、`2` 表示有待更新；真正写入必须加 `--yes`（可再加 `--skill`） |

`status` 默认做一次 fetch 探测（失败则标 `check failed`，不挡其它状态）。可用 `status --offline` 跳过网络。TUI 启动后**后台** fetch，不阻塞首屏；有结果再刷新顶栏。

因为 IDE 目录是 symlink，确认 update 后库文件一变，各 IDE 立刻看到新内容，不必再 `sync`（除非 symlink 缺失）。

探测结果可缓存在 `$AIKIT_HOME/cache/.update-check`（source → 远程 SHA + 检查时间），避免每次按键都 fetch；TUI/`status` 使用不超过 10 分钟的缓存，`update --check` 与确认后的 update 总是重新 fetch。

`aikit update <id> --ref <branch|tag|commit>` 用于显式换 pin 或回退到已知 commit。TTY 下显示当前 `ref/resolved` 与目标 ref 后确认；非 TTY 必须同时给 `--yes`。成功后写入新的 `ref/resolved`，失败则保留原 library 与账本值。

### 4.2 项目编辑

- `--add-agent X`：把 X 加入项目，校验有效视图后立即对账其项目目录
- `--remove-agent X`：先从新账本中移除 X 及其 `agent_bindings.X`，再拆除该项目 X 目录下的 aikit symlink；不删用户真目录
- `--name N`：只修改显示名和 CLI 标识，保持 path 与绑定
- `--path P`：用于项目目录已经移动后的重新绑定。旧 path 仍存在时先列出将拆除的 aikit symlink 并要求确认；之后更新账本并在新 path 对账。旧 path 不存在则直接更新并对账新 path
- 一次可组合多个参数，但必须先对最终状态做名称、真实 path 唯一性和跨层冲突校验

## 5. 存量

不在首次运行时静默扫描或迁移。

### 5.1 `scan` / `scan --adopt`

扫描范围：五个 Agent 的全局 skills 目录；若 cwd 已是登记项目或用户指定 `--project`，再加上该项目的项目级 skills 目录。

分类：

- 已是 aikit 自己的 symlink：不拷贝。`scan`（无 adopt）跳过；`scan --adopt` **只把该 id 写进对应 YAML 绑定**（避免随后 `sync` 因「期望无」拆链），不改 symlink
- 真目录：按 §2.1 本地 id 规则拷进 library
- 指向库外的 symlink：拷贝 **目标内容** 到 library，不在无 `--adopt` 的 scan 时改链

同名多处：hash 相同则库中一份，`--adopt` 时可 enable 到多个 Agent/项目；hash 不同则按 §2.1 本地 id（`local/<name>` 与 `local/<name>-<hash12>`），不用 agent/项目名做路径段。

非 TTY `--adopt --skill`：按**发现时的短名**或已解析 id 过滤（用户不必先 scan 一次才知道带 hash 后缀的新 id）。`--skill code-review` 匹配该短名的所有发现项。

`scan`：只入库，不改 IDE 目录、不写 agents/projects 绑定。

`scan --adopt` 顺序（与普通对账相反的「换真目录」只发生在这一步）：

1. 把选中项拷进 library（失败则停，原目录不动）
2. **先原子写 YAML**：全局发现项写入 `agents.<agent>`；项目发现项写入 `projects[].agent_bindings.<agent>`，绝不因为在一个 Agent 目录发现就写成项目公共绑定
3. **adopt 换链**：对该目标，若路径是真目录或库外 symlink，备份式替换为指向 library 的 symlink（先建新链到临时名，再替换）；若已是正确 aikit 链则跳过
4. 换链失败：原目录保留，YAML 已含绑定，`status` 记 conflict。普通 `sync` 仍不会覆盖真目录，因此**不会拆掉**这次失败留下的现场；用户修好权限后再次 `scan --adopt` 或对单条路径重试

非 TTY 的 `--adopt` 必须带 `--skill <id>`（可重复）或 `--all`，禁止无选择地全盘替换。

普通 `sync` **不会**把真目录改成 symlink。

### 5.2 `migrate`

显式一次性：

- `~/.aikit/catalog.yaml` 的 skills + `cache/` → 按已有 `source`+`name` 填 library（id 优于 `local/...`）
- 项目 `.aikit.yaml` 里的 `assets.skills` → `project add` 该目录。`targets` / Detect 结果映射到规范 Agent 名（`github-copilot` → `copilot`）；映射后为空则默认 `[cursor]`。旧项目 assets 原本面向全部 targets，因此迁入项目公共 `skills`，文件从 cache 来
- 不迁移 rules / mcp / commands
- 不删除旧文件；成功后打印可手删提示

## 6. 延后功能

第一版不提供 `export` / `import`，也不承诺跨机器复制或多机器同步。相关命令、TUI 入口、配置交换格式和测试全部延后；第一版只保证一台机器上的中央库与多个 IDE/项目工作区一致。

## 7. TUI（k9s 风格）与库选型

日常入口是 **全屏、可点选的终端界面**，交互对标 k9s / lazygit：一张表、光标移动、快捷键干活。不是一层层问卷，也不是 Web 页。

### 7.1 库

仍用 Charm（仓库已有 huh）。k9s 本身基于 tcell，本仓库不引入第二套 UI：

| 库 | 用途 |
|----|------|
| **bubbletea** | 全屏事件循环（主界面） |
| **bubbles** | `list` / `table` / `viewport` / `textinput` / `paginator` |
| **lipgloss** | 表头、选中行、底栏快捷键、更新角标 |
| **huh** | **仅**极少数 CLI 一次性表单；TUI 内用 bubbles `textinput` 浮层（安装 Git source 或本地 path） |

继续 **github.com/charmbracelet/\*** + bubbletea v1，不切 v2。业务逻辑不进 TUI 包。

### 7.2 布局（对标 k9s）

```
┌─ aikit  library │ agents │ projects │ presets │ status ── ↑ 3 updates ─┐
│ NAME            ID                              SOURCE        ON         │
│▸ code-review    vercel-labs/agent-skills/…      vercel-labs   cursor,cc  │
│  api-conv       your-org/xxx-skills/api-conv    your-org      aikit      │
│  archimate ↑    local/archimate                 local                    │
│                                                                          │
├─ code-review · vercel-labs/agent-skills · resolved a1b2c3d · behind 4 ─┤
│  SKILL.md preview / 已启用: cursor 全局, 项目 aikit                      │
├──────────────────────────────────────────────────────────────────────────┤
│  / filter   space toggle   e enable   u updates   s sync   ? help   q  │
└──────────────────────────────────────────────────────────────────────────┘
```

- **顶栏**：视图切换（类似 k9s 的 `:pod`）。`1` library、`2` agents、`3` projects、`4` presets、`5` status。有更新时右侧 `↑ N updates`。
- **主表**：`j/k` 或方向键移动，`enter` 进入详情或下一级（Agent → 该 Agent 的 skill 表；项目同理）。选中行高亮。
- **下半详情**：当前行的 id、source、`resolved`、落后 commit 数、启用位置。
- **底栏**：当前视图可用键，类似 k9s 提示条。
- `/` 过滤当前表；`?` 快捷键帮助；`q` / `esc` 返回或退出。

### 7.3 视图与按键

| 视图 | 表内容 | 主要操作 |
|------|--------|----------|
| **library** | 全部 skill | `d` 出库（有引用则确认 `--force` 语义）、`U` 更新当前行（仍走确认）、`r` 输入 ref/commit 回退、`i` 安装（Git source 或本地 path 输入浮层 → 新表勾选入库） |
| **agents** | 五个 Agent；enter 进入该 Agent 全局 skill 表 | 表内 `space` 切换 enable（立刻写 YAML+对账） |
| **projects** | 已登记项目；enter 后选择 `common` 或某个 Agent 的 skill 表 | `a` 登记 cwd；`e` 编辑名称/path/Agents；作用域表内 `space` 切换；`x` remove 项目 |
| **presets** | preset 列表；enter 成员表 | `n` 新建、`space` 改成员、`E` 套用；项目目标必须继续选 `common` 或具体 Agent |
| **status** | drift、scope-conflict、unmanaged、orphaned-link 与可更新列表 | `s` 执行 sync；`A` 对选中的 unmanaged 项进入 adopt 预览；`u` 进入更新确认表 |
| **updates**（`u`） | 落后于远端的 skill | 空格多选，`enter` 确认同步；`esc` 取消，**不写库** |

行尾 `↑`：该 skill 有远端更新。短名冲突 enable 时，详情区列出两个 id，必须先选一个。

扫描/adopt 挂在 library 的 `S` 键上，在当前 TUI 开子表，不退出到问卷向导。第一版没有 export/import 入口。

### 7.4 CLI 缺参数

子命令缺参数且为 TTY：进入**同一套全屏 TUI 的对应视图**（`aikit enable` → agents 或 projects 表），而不是 huh 问卷。非 TTY 仍报错退出。

## 8. 失败与 status

| 情况 | 行为 |
|------|------|
| enable 的 id 不在库中 | 拒绝，提示 `add` |
| preset 引用缺失 skill | enable/sync 拒绝，指出 preset 与 id |
| 短名冲突 | 该目标跳过，其余继续，非 0 退出 |
| 全局与项目同名不同 id | 写操作预校验拒绝；手改 YAML 则 status 报 `scope-conflict`，sync 跳过该项目 Agent |
| 库目录缺失 | 该条失败，提示 `update` 或重新 `add` |
| 项目 path 不存在 | 警告跳过 |
| 权限 / 建链失败 | 该条失败，继续其余，最后汇总 |
| remove 仍被引用 | 拒绝并列出引用；`--force` 才解绑出库 |
| add 时 id 已存在 | 拒绝；`--force` 覆盖文件 |
| scan 得到已有同 id | 跳过拷贝；`--adopt` 仍按发现位置写绑定 |

`status` 每次检查五个 Agent 的全局 skills 目录和所有 path 存在的登记项目目录，并保持只读：

- `missing`：YAML 期望存在但链接缺失
- `conflict`：期望位置被真目录、真文件或库外 symlink 占用
- `scope-conflict`：同一「项目 × Agent」的全局与项目层出现同名不同 id
- `unmanaged`：目标 skills 目录中存在未进入账本、也不占据某个期望路径的真目录、真文件或库外 symlink；可交给 `scan --adopt`。占据期望路径的同类对象只归为 `conflict`，不重复计数
- `orphaned-link`：指向 aikit library、但账本没有对应绑定的 symlink；普通 `sync` 会拆除，用户也可先 `scan --adopt` 恢复绑定
- `updates`：远端可更新数量；只提示，不执行更新

`status` 示例：

```
Library: 51 skills
Agents: cursor 12/12  claude-code 8/8
Projects: aikit 3/3  (path ok)
Updates: 3 (code-review a1b2c3d..f4e5d6, …)
Unmanaged: 2
  - ~/.cursor/skills/private-helper (real directory)
  - ~/work/aikit/.codex/skills/local-debug (external symlink)
Drift:
  - missing: ~/.cursor/skills/archimate
  - conflict: ~/work/aikit/.cursor/skills/code-review (real directory, not symlink)
  - scope-conflict: aikit/cursor code-review (source-a/code-review vs source-b/code-review)
```

## 9. 测试

不连真实 Cursor / Claude。对账引擎用临时目录。

必须覆盖：

- 缺链建、正确跳过、错误库内链改写、多余 aikit 链拆除
- 真目录与库外 symlink：不删不覆盖，记冲突
- 短名冲突：该目标不改盘
- 全局和项目同名同 id：只建全局链接；关闭全局后自动补项目链接
- 全局和项目同名不同 id：enable/preset/project edit 在写 YAML 前拒绝；手改 YAML 后 status/sync 报错
- `--dry-run` 不写盘
- Git source 与本地单 skill/多 skill 路径均可 add；纯 add 不碰 IDE 目录
- 本地 add 复制后删除原路径不影响 library
- `enable`/`disable`：YAML 先于磁盘；写盘失败后 status 有 drift，sync 能修
- `enable --project P --agent X` 只改项目 X；只 `--agent X` 只改全局；只 `--project P` 改项目公共集合
- `remove` 引用保护与 `--force`
- `scan` 只入库；`scan --adopt` 为拷贝 → 写 YAML → 换链；换链失败则 conflict，且随后普通 sync 不拆真目录
- `project remove`：先删 YAML 再拆该项目 aikit 链
- `project edit`：增删 Agent、改名、项目移动后的 path 重绑；删除 Agent 时只拆该 Agent 的项目链接
- `sync --agent` 只动该 Agent 全局目录
- 已有 aikit symlink 的 `--adopt` 只写 YAML 不改链；随后普通 `sync` 因「期望有」而保留
- 在已登记项目目录执行无参 `sync` / `status` 仍是全量
- 存量 hash 相同只入一份并多处 enable；不同则 `local/<name>` 与 `local/<name>-<hash12>`
- `migrate`：catalog + 项目 `.aikit.yaml` 的 skills 进入新 config，旧文件仍在
- CLI 冒烟：`add` → `enable --agent` → `project add` → `enable --project` → `status` 全绿 → `disable` 断链
- `update --check`：远端 HEAD ≠ `resolved` 列为可更新；pin 的 tag 不列入
- TTY `update` 拒绝确认时库文件与 `resolved` 不变
- 非 TTY `update` 无 `--yes` 不写库；`--yes` 才更新
- `update <id> --ref <commit>` 可回退；失败保持原 ref/resolved 和 library
- TTY `status` 发现更新只提示，不进入确认、不写库
- `status --offline` 不发起 fetch
- `status` 列出 unmanaged/orphaned-link；选中 unmanaged 后可进入精确 adopt

TUI：逻辑测底层 API；k9s 式主界面做一次手动走通（列表移动、项目 common/Agent 作用域切换、space enable、项目编辑、unmanaged adopt、`u` 确认更新）。CI 不模拟按键。

## 10. 实现边界

### 10.1 废弃现有代码

第一版按本文**重写**，下列现有实现均可删除或不再编译进二进制，不必做兼容层、功能开关或「旧命令还在但提示迁移」：

- `cmd/` 下现有 `init` / `add` / `sync` / `catalog*` / `publish` / `catalog ui` 等子命令
- 项目级 `.aikit.yaml` 作为账本的读写与 copy 安装
- `internal/catalog`、现有 catalog Web（gin + frontend）
- 现有 Agent `Install*` copy 逻辑

`aikit migrate` 只**读**用户机器上已经存在的旧文件（`catalog.yaml`、项目 `.aikit.yaml`、cache），不依赖旧 Go 包继续提供命令。读完写入新 `config.yaml` + library 即可。

模块路径、`main.go`、go.mod 模块名可以保留，避免无谓改 import。

### 10.2 新包划分

实现计划阶段可再拆文件，语义保持不变：

- `pkg/config`：读写 `config.yaml`
- `internal/library`：入库、更新、id 分配、hash
- `internal/link`：期望集合 + 普通对账 + adopt 换链（唯一写 IDE 目录的地方）
- `internal/agent`：只提供规范 Name + 全局/项目 skills 路径；`github-copilot` → `copilot`
- `internal/status`：只读汇总 drift、scope-conflict、unmanaged、orphaned-link 与 updatecheck 结果
- `cmd/`：cobra；TTY 缺参数打开对应 TUI 视图
- `internal/tui`：bubbletea 全屏（k9s 式表 + 快捷键）；huh 仅用于非全屏的一次性 CLI 表单（若仍需要）
- `internal/updatecheck`：fetch、对比 `resolved`、确认后调用 library update

产品语义以本文为准。
