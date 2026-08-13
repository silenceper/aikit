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
├── config.lock                 # 单机进程互斥锁
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
  - 其它 git URL：规范化为包含 host 与完整仓库 path 的稳定 source，再拼 `name`；不能丢弃 GitLab subgroup 等中间路径
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
      source_path: skills/code-review # skill 在仓库内的相对目录
      ref:
        kind: tag                     # branch / tag / commit
        value: v1.2.0
      resolved: 0123456789abcdef0123456789abcdef01234567 # 完整 Git object id
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

pending_operations: []               # aikit 内部恢复日志，正常稳定态为空
```

Agent 全局有效集合 = `agents.<agent>.presets` 展开 ∪ `agents.<agent>.skills`。

项目公共集合 = `projects[].presets` 展开 ∪ `projects[].skills`，应用到该项目 `agents` 中的每个 Agent。项目中特定 Agent 的目标集合 = 项目公共集合 ∪ `projects[].agent_bindings.<agent>.presets` 展开 ∪ `projects[].agent_bindings.<agent>.skills`。`agent_bindings` 的 key 必须同时出现在该项目的 `agents` 中。

所有层级都只保存 preset **名**（不展开）；修改 preset 后，所有引用该 preset 的全局与项目目标在下次 `sync` 时一起更新。一个 skill 同时被直接引用和 preset 引用时按集合去重。

`projects[].path` 为本机绝对路径。`projects[].name` 在配置内唯一，用于 CLI/TUI 与 `--project`；规范化后的真实 path 也必须唯一，禁止同一路径登记成两个项目。`presets[].name` 同样唯一。

远程 library 条目必须保存 `source_path`，它是发现到的 `SKILL.md` 所在目录相对于仓库根的路径。一个仓库内多个或嵌套 skill 各自保存自己的 `source_path`；update 只从该路径重拷对应 skill。`ref.kind` 明确区分 branch/tag/commit，禁止靠名称猜类型；用户未指定 ref 时，add 解析远程默认分支并显式写成 `kind: branch`。`resolved` 始终保存 Git 返回的完整 object id，界面可缩写显示但比较时不得使用短 SHA。

`pending_operations` 是仍属于账本的一部分的恢复日志，只允许 aikit 写入：

- `cleanup`：保存逻辑 scope、旧目标链接的绝对 path、预期 id 和原因，用于 project remove / remove-agent / path rebind 后可重试地拆链
- `adopt`：保存逻辑 scope、目标 path、library id、临时 symlink path、backup path，以及原对象的 kind+hash（外部 symlink 则记录原 link target），表示用户已经授权把该对象收归

cleanup 重试时，path 不存在视为完成；path 仍是指向记录 id 的 aikit symlink 才删除；若已变成真目录、库外 symlink 或不同 id，停止并报 `pending-cleanup conflict`，不能删除后来出现的用户内容。

变更命令在执行新变更前，只恢复其将要触碰的逻辑 scope 内的 pending operation。`sync` 同样按 §3.3 的 scope 过滤：无参数处理全部；只 `--agent X` 仅处理 X 全局 scope；只 `--project P` 处理 P 的项目 scopes；两者都有则只处理 `P × X`。已删除项目留下的 cleanup 只能由无参数全量 sync 或触发删除的原命令继续处理。其它 scope 的 pending operation 只由 `status` 提示，不能被一次窄范围命令顺带修改。

`sync --dry-run` 在任何情况下都不推进、不恢复、不清除 pending operation，只打印按当前 scope 将执行的恢复和对账计划。非 dry-run 操作完成后原子删除对应记录；失败则保留记录并由 `status` 展示。

所有变更命令从读取 config 到更新 YAML、对账和清除 pending operation 全程持有 `$AIKIT_HOME/config.lock` 独占锁，避免 TUI 与 CLI 并发覆盖账本；获得锁后必须重新读取 config。

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

### 2.4 路径与内容边界

- normalized source 保留小写 host、非默认 port 和完整 repo path；每个 segment 做确定性的 UTF-8 百分号编码。GitHub shorthand 与等价 GitHub URL 得到同一值，不同完整 URL 不得因只取末两段而碰撞
- skill name 必须非空，不能是 `.` / `..`，不能包含路径分隔符或 NUL；不合法的 frontmatter name 直接拒绝入库，不退化成文件路径
- `source_path` 必须是 clean 的仓库内相对路径，不能是绝对路径或包含 `..`；解析 symlink 后也必须仍在 cache checkout 内
- 所有 cache/library/IDE 目标在 join 后都用 `filepath.Rel`（或等价逻辑）验证仍位于预期根目录，验证失败时停止，不能读写根目录外路径
- 复制和 hash 遇到 skill 内部 symlink 时只接受解析后仍在该 skill 根内的目标；越界 symlink 拒绝入库，不能把机器上的其它文件复制进 library
- hash 按排序后的记录计算，每条记录包含相对路径长度+路径、类型、可执行位、内容长度+内容，避免简单字符串拼接歧义

## 3. 对账引擎

`enable` / `disable` / `sync` / `project remove` 走 **普通对账**。`scan --adopt` 走 **adopt 换链**（§5.1），不套用下表「真目录不覆盖」。

「由 aikit 管理」= 该路径是 symlink，且最终目标在 `$AIKIT_HOME/library/skills/` 之下。

### 3.1 普通对账

| 磁盘现状 | YAML 期望有 | YAML 期望无 |
|----------|-------------|-------------|
| 无此路径 | 建 symlink | 不动 |
| 正确指向该 id 的 symlink | 跳过 | 断链 |
| symlink 指向库内另一 id | 改指向 | 断链 |
| 断链，词法目标是期望 id，但 library 目录缺失 | 不改链，报 `library-missing` | 断链 |
| 断链，词法目标是库内另一 id | 期望 library 存在时改指向，否则报 `library-missing` | 断链 |
| 断链，词法目标在 library 外 | 不覆盖，记冲突 | 不删 |
| 真目录 / 真文件 | 不覆盖，记冲突，其它路径继续 | 不删 |
| 指向库外的 symlink | 不覆盖，记冲突 | 不删 |

写盘顺序：**先原子写 YAML**（temp + rename），再对账磁盘。写盘失败时账本已是新状态，`status` 显示 drift，再 `sync` 修复。禁止「目录已变、账本未记」。

断链归属用 `Lstat + Readlink` 做词法判断：相对目标先相对 symlink 所在目录转为绝对 clean path，再检查是否严格位于 library 根下；不能用会因目标不存在而失败的 `EvalSymlinks` 来判断。expected library 目录不存在时不得创建一个永久断链，必须保留现场并报 `library-missing`，提示 `update --force` 或重新 add。

`sync --dry-run` 只打印将建 / 将拆 / 冲突以及范围内 pending recovery 计划，绝不改盘或改 YAML。

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

- 只 `--agent X`：改 `agents.X` 全局绑定，并对账 X 的全局目录以及所有声明 X 的登记项目。原因是同 id 的项目链接可能需要在全局启用后收起、或在全局禁用后补建
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
| `aikit project edit <name>` | `--name` / `--path` / `--add-agent` / `--remove-agent` / `--yes`；见 §4.2 |
| `aikit project remove <name>` | 从 YAML 删除项目的同时写入 cleanup operations，再拆除其 aikit symlink；全部成功后清除 operations。不删用户真目录 |
| `aikit status` | 只读：库数量、绑定、drift、未纳管项、可更新数；不会询问或执行 update |
| `aikit update --check` | 只探测，不写库；打印落后条目 |
| `aikit sync` | 按 YAML 全量对账（只对账 symlink，**不**拉远端） |
| `aikit migrate` | 见 §5.2 |

`add` 时 id 已存在：拒绝；`--force` 覆盖库文件，保留已有绑定。

`enable` 的 id 不在 library、或 preset 引用缺失 skill：拒绝，不改盘。

`aikit update` 与 `ref`：

- `ref.kind=branch`：`git fetch` 后更新到 `ref.value` 的远程 HEAD，根据 `source_path` 把该 skill 目录原子重拷到 library，写入新的完整 `resolved`
- `ref.kind=tag|commit`：默认不前进，打印 pinned 并跳过；`--force` 仍检出**同一个** pin，再根据 `source_path` 重拷（用于修复库文件损坏，不会升到更新的 tag）
- 无远程 `source` 的本地导入（id 以 `local/` 开头）：`update` 跳过
- 改 pin 或回退：使用 `aikit update <id> --ref branch:<name>|tag:<name>|commit:<object-id>`；类型前缀必填，不要求用户手改 YAML
- `disable <id>`：只从该目标的 `skills[]` 去掉该 id。若有效集合仍包含它（来自 preset），磁盘不变并打印提示；若用户就是要从 preset 里拿掉，应改 preset，而不是 disable

本地路径 `add`：

- 参数在本机存在时按 path 处理，否则按 Git source 处理；不存在且也不是合法 Git source 时报错
- path 根目录有 `SKILL.md` 时视为单个 skill；否则递归发现其下所有 `SKILL.md`
- 内容复制到 library 后不再依赖原 path；library 条目省略远程 `source/ref/resolved`，记录内容 `hash`
- `add` 只入库；只有显式给 `--agent` / `--project` 才继续 enable

### 4.1 远端更新探测与确认

「有更新」= 该 skill 有 git `source`，且 `ref.kind=branch`，`git fetch` 后远程该 branch 的完整 HEAD object id ≠ `resolved`。pinned tag/commit 不参与「有更新」提示（除非用户 `update --force` 修文件）。

**不自动写入。** 探测到更新后必须确认：

| 环境 | 行为 |
|------|------|
| TUI | 顶栏显示 `↑ 3 updates`；`u` 打开可更新列表，空格勾选，`enter` 确认后才 `update` 选中项 |
| TTY 的 `aikit update` 发现更新 | 列出 id 与新旧 SHA，问是否同步（可多选）；否或 Ctrl-C 则不写库 |
| TTY 的 `aikit status` 发现更新 | 只打印可更新项和建议命令 `aikit update`，不询问、不写库 |
| 非 TTY | `update` 只探测并打印，退出码 0 表示无更新、`2` 表示有待更新；真正写入必须加 `--yes`（可再加 `--skill`） |

`status` 默认做一次 fetch 探测（失败则标 `check failed`，不挡其它状态）。可用 `status --offline` 跳过网络。TUI 启动后**后台** fetch，不阻塞首屏；有结果再刷新顶栏。

因为 IDE 目录是 symlink，确认 update 后库文件一变，各 IDE 立刻看到新内容，不必再 `sync`（除非 symlink 缺失）。

探测结果可缓存在 `$AIKIT_HOME/cache/.update-check`，key 必须是 `(canonical source, ref.kind, ref.value)`，value 为该 ref 的远程完整 object id + 检查时间；同仓库不同 branch 不能共用结果。TUI/`status` 使用不超过 10 分钟的缓存，`update --check` 与确认后的 update 总是重新 fetch。

`aikit update <id> --ref branch:<name>|tag:<name>|commit:<object-id>` 用于显式换 pin 或回退到已知 commit。TTY 下显示当前 `ref/resolved` 与目标 ref 后确认；非 TTY 必须同时给 `--yes`。成功后写入新的结构化 `ref` 与完整 `resolved`，失败则保留原 library 与账本值。目标 ref 下的 `source_path/SKILL.md` 不存在或 name 改变时拒绝更新并提示重新 add，不能悄悄更新到另一个 skill。

### 4.2 项目编辑

- `--add-agent X`：把 X 加入项目，校验有效视图后立即对账其项目目录
- `--remove-agent X`：同一次 YAML 写入中移除 X 及其 `agent_bindings.X`，并为原有效集合的每条项目链接写入 cleanup operation；随后执行 cleanup，不删用户真目录。中断后 `sync` 根据 operation 继续
- `--name N`：只修改显示名和 CLI 标识，保持 path 与绑定
- `--path P`：用于项目目录已经移动后的重新绑定。旧 path 仍存在时先预览要拆除的旧项目 aikit symlink 并要求确认；确认后一次原子 YAML 写入同时更新 path、为每条旧链接加入 cleanup operation。随后先处理旧 path cleanup，再对账新 path，最后删除 operation。任一步失败都保留 operation，`status` 报 `pending-cleanup`，下次 `sync` 继续；旧 path 不存在时无需 cleanup，直接更新并对账新 path
- 一次可组合多个参数，但必须先对最终状态做名称、真实 path 唯一性和跨层冲突校验

TTY 下，只要 `--path` 会改变 canonical path 且旧 path 仍存在，就显示 cleanup/new-target 预览并确认；`--yes` 可跳过确认。非 TTY 下任何实际 path 变更都必须显式给 `--yes`，否则在写 YAML 前报错退出，绝不等待输入或静默重绑。只改 name/Agents 且参数完整时不需要 `--yes`。

### 4.3 退出码

- `0`：请求的操作全部完成；`status` 无 drift/conflict/pending（只有 updates 仍为 0）
- `1`：参数/IO 错误、任一目标部分失败、pending operation 冲突、migrate 冲突，或 `status` 发现 drift/conflict/pending/check failed
- `2`：只用于 `update --check` 或非 TTY 无 `--yes` 的 update，表示发现待更新项但没有写库

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
2. 在目标的同一父目录选定唯一的临时路径与 backup 路径；记录原对象的 kind+hash（外部 symlink 记录 link target）；**先原子写 YAML**：全局发现项写入 `agents.<agent>`，项目发现项写入 `projects[].agent_bindings.<agent>`，同时写入完整的 `pending_operations.adopt`
3. 在 temp path 创建指向 library id 的 symlink 并验证；失败则删除 temp、保留原目录和 pending operation
4. 把原真目录或库外 symlink 原子 rename 到 backup；失败则删除 temp，原目录保持不变
5. 把 temp symlink 原子 rename 到正式 target；失败则立即把 backup rename 回 target。回滚成功后保留 pending operation 供重试；回滚也失败时绝不删除 backup，`status` 报 `adopt-recovery` 并显示 backup path
6. 验证正式 target 正确指向 library，且 backup 内容 hash 与已入库副本一致；然后删除 backup，最后原子删除 pending operation

temp/backup 使用 aikit 专用前缀加随机 nonce，不覆盖任何已有路径；冲突就重新生成。恢复时以 pending 中的原对象指纹和实际磁盘组合判断：

| 磁盘组合 | 恢复动作 |
|----------|----------|
| target 仍匹配原对象，backup 不存在 | 这是 temp 创建失败、原对象 rename 失败或成功回滚后的可重试态；temp 不存在则从步骤 3 重建，temp 是正确 symlink则复用；temp 被其它对象占用则走最后一行人工恢复 |
| target 不存在，backup 匹配原对象，temp 是正确 symlink | 从步骤 5 继续，把 temp rename 到 target |
| target 不存在，backup 匹配原对象，temp 不存在 | 先把 backup 恢复到 target，再回到上一行的“原对象仍在”状态重试 |
| target 是正确 aikit symlink，backup 匹配原对象 | 校验 hash 后删除 backup并完成 operation |
| target 是正确 aikit symlink，backup 不存在 | operation 已完成，清除记录 |
| 任一路径存在但类型、id、link target 或原对象 hash 不符合记录 | 停止并报 `adopt-recovery`，显示 target/temp/backup；不猜测、不覆盖、不删除 |

因此 temp 创建失败、原对象 rename 失败、以及第二次 rename 失败但回滚成功都能由后续 `sync` 重试。只有磁盘对象已被外部修改或回滚失败造成不一致时才进入人工恢复。

换链失败时 YAML 已含绑定和 pending operation，普通对账不得把该目标当普通真目录冲突处理，必须先走上述恢复流程。用户修好权限后执行 `sync` 或再次 `scan --adopt` 即可重试。

非 TTY 的 `--adopt` 必须带 `--skill <id>`（可重复）或 `--all`，禁止无选择地全盘替换。

普通 `sync` **不会**把真目录改成 symlink。

### 5.2 `migrate`

迁移只处理用户明确提供或当前已知的位置，不扫描整个磁盘：

- `aikit migrate`：读取 `$AIKIT_HOME/catalog.yaml`；若 cwd 下存在 `.aikit.yaml`，同时迁移 cwd 项目
- `aikit migrate --project <path>`：可重复，迁移指定项目的 `.aikit.yaml`
- `aikit migrate --dry-run`：只显示将入库、合并、跳过和冲突的条目
- `aikit migrate --adopt`：除迁入账本外，还明确授权把旧项目/IDE 真实 skill 目录按 §5.1 收归；不带该参数时绝不替换现场

合并与幂等规则：

- config 不存在则创建；已有非空 config 时只合并，不整体覆盖
- `catalog.yaml` 的 skill 根据旧 `source` + name 在 cache 中重新发现，写入新的 source/source_path/ref/resolved；同 id 且内容相同则跳过
- 同一旧 source 中发现多个相同 name、无法唯一确定 source_path 时，该条冲突并跳过，要求用户迁移后用 `add <source-or-path>` 明确重新添加；不得任选第一个
- 同 id 但 source_path 或内容不同：该条冲突并跳过，不覆盖现有 library；其它条目继续，最终退出码非 0
- 项目 canonical path 已登记：把旧 targets 映射后并入 `agents`，旧 `assets.skills` 并入项目公共 `skills`
- 项目 path 未登记但 name 与另一 path 冲突：该项目跳过并报错，不覆盖同名项目
- `targets` / Detect 结果映射到规范 Agent 名（`github-copilot` → `copilot`）；映射后为空则默认 `[cursor]`
- rules / mcp / commands 不迁移；旧文件、旧 catalog、旧 `.aikit.yaml` 和 cache 永不删除

每个 library 条目必须先成功复制并校验，才可在同一次原子 YAML 写入中加入该条目及其引用。单个条目失败不会留下悬空引用；已经成功迁入的条目保留，因此重复执行可继续完成剩余项。

默认 migrate 写入项目绑定后，如果旧 IDE 位置是真目录，`status` 会显示 conflict，普通 sync 不覆盖；用户随后运行 `scan --adopt`。带 `--adopt` 时，迁移直接为这些已授权位置创建 pending adopt operations并执行 §5.1 流程。迁移结束必须打印“已纳管 / 待 adopt / 冲突 / 跳过”汇总。

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
| 库目录缺失 | 不建/改 symlink，报 `library-missing`，提示 `update --force` 或重新 `add` |
| 项目 path 不存在 | 警告跳过 |
| 权限 / 建链失败 | 该条失败，继续其余，最后汇总 |
| remove 仍被引用 | 拒绝并列出引用；`--force` 才解绑出库 |
| add 时 id 已存在 | 拒绝；`--force` 覆盖文件 |
| scan 得到已有同 id | 跳过拷贝；`--adopt` 仍按发现位置写绑定 |

`status` 每次检查五个 Agent 的全局 skills 目录和所有 path 存在的登记项目目录，并保持只读：

- `missing`：YAML 期望存在但链接缺失
- `library-missing`：YAML 引用的 library id 目录缺失；与普通链接缺失分开报告
- `conflict`：期望位置被真目录、真文件或库外 symlink 占用
- `scope-conflict`：同一「项目 × Agent」的全局与项目层出现同名不同 id
- `unmanaged`：目标 skills 目录中存在未进入账本、也不占据某个期望路径的真目录、真文件或库外 symlink；可交给 `scan --adopt`。占据期望路径的同类对象只归为 `conflict`，不重复计数
- `orphaned-link`：指向 aikit library、但账本没有对应绑定的 symlink；普通 `sync` 会拆除，用户也可先 `scan --adopt` 恢复绑定
- `pending-cleanup` / `adopt-recovery`：持久化 operation 尚未完成；显示 target、backup（若有）和下一步 `sync`/人工处理建议
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
  - library-missing: local/old-helper
  - conflict: ~/work/aikit/.cursor/skills/code-review (real directory, not symlink)
  - scope-conflict: aikit/cursor code-review (source-a/code-review vs source-b/code-review)
```

## 9. 测试

不连真实 Cursor / Claude。对账引擎用临时目录。

必须覆盖：

- 缺链建、正确跳过、错误库内链改写、多余 aikit 链拆除
- 真目录与库外 symlink：不删不覆盖，记冲突；库内/库外 broken symlink 分别处理
- expected library 目录缺失时报 `library-missing`，不创建永久断链
- 短名冲突：该目标不改盘
- 全局和项目同名同 id：只建全局链接；关闭全局后自动补项目链接
- 全局和项目同名不同 id：enable/preset/project edit 在写 YAML 前拒绝；手改 YAML 后 status/sync 报错
- `--dry-run` 不写盘
- `sync --dry-run` 有 pending operation 时仍绝对只读，只打印恢复计划
- Git source 与本地单 skill/多 skill 路径均可 add；纯 add 不碰 IDE 目录
- 本地 add 复制后删除原路径不影响 library
- source/name/source_path 的 `..`、绝对路径、分隔符与越界 symlink 被拒绝；嵌套 Git group 不发生 source id 碰撞
- `enable`/`disable`：YAML 先于磁盘；写盘失败后 status 有 drift，sync 能修
- `enable --project P --agent X` 只改项目 X；只 `--agent X` 只改全局；只 `--project P` 改项目公共集合
- 全局 enable/disable 后重算所有声明该 Agent 的项目有效视图：启用同 id 时拆除冗余项目链，关闭时补建仍有项目绑定的链
- `remove` 引用保护与 `--force`
- `scan` 只入库；`scan --adopt` 为拷贝 → 写 YAML → 换链；换链失败则 conflict，且随后普通 sync 不拆真目录
- `project remove`：YAML 删除项目并写 cleanup operations；中断后 sync 继续拆链并清 operation
- `project edit`：增删 Agent、改名、项目移动后的 path 重绑；旧 path cleanup 中断后可恢复；删除 Agent 时只拆该 Agent 的项目链接
- 非 TTY `project edit --path` 无 `--yes` 在写盘前失败；带 `--yes` 才重绑
- `sync --agent` 只动该 Agent 全局目录
- scoped sync 只推进范围内 pending operation，不修改其它 Agent/项目的 recovery 状态
- 已有 aikit symlink 的 `--adopt` 只写 YAML 不改链；随后普通 `sync` 因「期望有」而保留
- 在已登记项目目录执行无参 `sync` / `status` 仍是全量
- 存量 hash 相同只入一份并多处 enable；不同则 `local/<name>` 与 `local/<name>-<hash12>`
- `migrate`：catalog + 显式项目进入空/非空 config；重复执行幂等；冲突不覆盖；默认待 adopt，`--adopt` 走恢复日志；旧文件仍在
- CLI 冒烟：`add` → `enable --agent` → `project add` → `enable --project` → `status` 全绿 → `disable` 断链
- 多 skill/嵌套 skill 仓库分别保存 source_path，update 只重拷目标路径
- `update --check`：远端完整 HEAD object id ≠ `resolved` 列为可更新；pin 的 tag 不列入；branch/tag 同名按 ref.kind 区分
- 同一 source 的两个 branch 使用独立 update-check cache key，不串用远端 SHA
- TTY `update` 拒绝确认时库文件与 `resolved` 不变
- 非 TTY `update` 无 `--yes` 不写库；`--yes` 才更新
- `update <id> --ref <commit>` 可回退；失败保持原 ref/resolved 和 library
- TTY `status` 发现更新只提示，不进入确认、不写库
- `status --offline` 不发起 fetch
- `status` 列出 unmanaged/orphaned-link；选中 unmanaged 后可进入精确 adopt
- adopt 在原目录 rename 后、临时链 rename 前中断时可恢复；backup 冲突或回滚失败时不删除用户数据
- adopt 在 temp 创建失败、原对象 rename 失败、第二次 rename 失败并成功回滚三种状态下，后续 sync 均能重试
- 两个并发变更进程由 config.lock 串行，后获得锁者重读账本，不丢更新

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
