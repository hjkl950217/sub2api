# AGENTS.md — sub2api fork

本仓库是 **Sub2API 的个人二次开发 fork**（origin `hjkl950217/sub2api`）。前端补丁与后端二开的管理中心在 PersonalAITaskCenter 的 `工具/管理_sub2api/子任务/二开sub2api/`，本文件只描述**本仓库自身**的改动与纪律。

## 1. 唯一认可的上游

**`https://github.com/Wei-Shaw/sub2api`（`Wei-Shaw/sub2api`）是唯一认可的官方上游。**

- 本仓库的代码、版本号、发布产物、更新检查，全部以该仓库为基准；
- 任何其他同名或同源仓库（包括各种 fork、镜像站、二次分发的仓库）都不是上游，不参与合并、不对齐版本、不作为更新来源；
- 合并上游更新一律走 `git fetch upstream && git merge upstream/main`，`upstream` remote 指向上述地址；
- 版本号跟随上游，**不自行改号**。

```bash
git remote -v
# origin    https://github.com/hjkl950217/sub2api.git   （本 fork，推送目标）
# upstream  https://github.com/Wei-Shaw/sub2api.git     （唯一上游，只读）
```

## 2. 相对上游的改动清单

改动都提交在 `main` 分支。合并上游后，按本节逐条复查是否还在、是否仍然需要。

本文件本身也是改动之一：上游 `.gitignore:122` 忽略了 `CLAUDE.md`。2026-10-15 起本仓库的这份说明改名 `AGENTS.md`（本机约定：项目级指令文件统一叫 `AGENTS.md`），改名后不再被 `.gitignore` 命中，**不需要再 `git add -f`**。

```bash
git add AGENTS.md
```

### 2.0 标记纪律（强制，长空 2026-09-30 定）

**所有改动过的上游源码文件——前端和后端都算——必须在每处改动的紧邻位置带一个 `FORK-ANCHOR` 标记。**

```
// FORK-ANCHOR: <唯一名字> (<一句话说明这处改了什么>)
```

- 后端用 `//`，前端 `.vue/.ts` 用 `//`（模板里用 `<!-- FORK-ANCHOR: ... -->`），YAML 用 `#`；
- 一个标记对应一处**改动点**，不是整个文件一处；同一文件改了 3 处就要有 3 个标记；
- 标记名全局唯一，用下面这条命令列出全部改动点（**只数 git 已跟踪文件**，所以中文名文档和 `node_modules` 里的缓存都不会干扰；`.md` 文档里只是在列举锚点名，不算改动点）：

  ```bash
  git -c core.quotepath=false grep -o "FORK-ANCHOR:" -- . | grep -v '\.md:' | wc -l
  ```

  `-c core.quotepath=false` 不能省：中文文件名的路径会被 git 转义成八进制，`grep -v '\.md:'` 就匹配不上，会多算。

- **新增的整文件**（`fork/` 下的东西、新增的视图/测试文件）不算「改上游」，不强制带标记，但文件头要有一行 `FORK:` 说明；
- 合并上游后先跑上面那条命令对数量（当前 **722 个** = 源码 702 + `.github/` 20），数量变少就是有改动被上游覆盖或冲突时被丢掉了。
  其中 `.github/` 占 15 个，单独看：`git -c core.quotepath=false grep -o "FORK-ANCHOR:" -- .github | wc -l`。

全量清单见 **`锚点清单.md`**：按文件分组列出每个锚点的名字，合并上游后照它逐条核对。

### 2.1 `.github/workflows/release.yml`

| # | 位置 | 改动 | 原因 |
|---|---|---|---|
| 1 | 文件头注释 | 加了 `# FORK` 说明块，列出本文件的三条改动 | 合并上游后一眼看到要复查什么 |
| 2 | `workflow_dispatch.inputs` | 新增 `use_version_file`（boolean，默认 false）；`tag` 改为非必填 | 支持无 tag 手动发版 |
| 3 | `prepare` 作业的 plan 步骤 | 传入 `USE_VERSION_FILE`，为真时给 `release_matrix.py plan` 加 `--version-file` | 同上 |
| 4 | `release` 作业 | **删除** `Login to DockerHub` 步骤 | 只推 GHCR |
| 5 | `release` 作业 | **删除** `Update DockerHub description` 步骤 | 同上 |
| 6 | `build-binaries` / `release` 两处 env | `DOCKERHUB_USERNAME` 从 `secrets.DOCKERHUB_USERNAME \|\| 'skip'` 改为固定 `skip` | 不依赖 secret，渠道恒定失效 |
| 7 | Telegram 通知步骤 env | `DOCKERHUB_USERNAME` 从 secret 改为空串 `''` | 该脚本用 `[ -n "$DOCKERHUB_USERNAME" ]` 判断，`skip` 会被当成"有值"而输出 `skip/sub2api` 脏命令，必须留空 |
| 8 | `sync-version-file` 作业 | `if` 改为 `false`，整作业停用 | 该作业会把发版版本号 commit 回默认分支，导致 `main` 与上游分叉，`git pull --ff-only` 从此失败 |
| 9 | `build-frontend` 作业 | 新增两步：`python fork/frontend/patch_dist.py` 把两处前端补丁打进 dist，再跑 `node fork/frontend/test_patched_dist.mjs` | 让镜像自带前端补丁，部署后不必再往 `data/public/` 放覆盖文件 |
| 10 | `release` 作业 | 新增 `Reset release tag and release` 步骤（非 dry-run）：`git tag -f` + `git push --force` + `gh release delete` | 同名 tag / Release 一律重建，保证 tag 指向本次发版的提交 |
| 11 | `build-frontend` 作业 | 新增 `Fork embed override tests`：`go test -tags=embed -run TestFork ./internal/web/`，并校验恰好 2 个用例通过 | 上游 CI 只跑 `-tags=unit`，不编译 embed 代码，二开的覆盖改动不会被它覆盖，必须在这里把关；`-run` 匹配不到用例时 `go test` 也返回 0，必须核对数量 |

上表 11 条对应的锚点在 `.github/workflows/release.yml` 里共 13 处（第 6、7 条各涉及多处 env），锚点全量清单见 `锚点清单.md`。

### 2.2 `.github/release-tools/release_matrix.py`

`plan()` 新增 `--version-file` 模式，并在文件头加了 `FORK` 说明：

- 版本取所选 ref 上的 `backend/cmd/server/VERSION`；
- tag 优先取 HEAD 上已存在的 `v*` tag；没有则回退为 `v<version>`；
- 该模式下不再要求 checkout 与 tag 的 commit 一致。

原因：上游的 `plan()` 强制要求 `ref` 是 `v*` tag 且 tag 指向当前 commit，改完代码后必须先打 tag 才能发版。`--version-file` 让"打完补丁直接手动触发"成为可能。

### 2.3 `fork/frontend/`（新增目录）

镜像内嵌前端补丁的实现，是 `工具/管理_sub2api/子任务/二开sub2api/旧补丁/前端补丁/` 那套「运行时覆盖」方案的构建期等价物。

**当前是两处补丁**（2026-09-30 起；「面板注入」已删，见 2.6c）：

| 文件 | 作用 |
|---|---|
| `patch_dist.py` | 把补丁打进 `dist`：入口 `getAvailableModels` 改 local-first、渠道监控列表按 ID 升序。每个模式必须命中且仅命中一次，否则失败退出；已打过补丁的 dist 会被拒绝重复打 |
| `test_patched_dist.mjs` | 补丁后的 dist 单测：local-first 行为断言 + 监控排序标记 + 2 处 DOM 锚点 + 账号管理默认 ID 降序的 3 条断言 |

入口 chunk 必须从 `dist/index.html` 读：`assets/` 下还有别的 `index-*.js` 路由分包，按文件名通配会命中 4 个。

### 2.4 `.github/release-tools/test_release_matrix.py`

新增 3 个用例覆盖 `--version-file`：

- `test_version_file_plan_publishes_without_a_tag`：无 tag 时 tag 回退为 `v<version>`；
- `test_version_file_plan_prefers_an_existing_tag`：HEAD 上有 tag 时用真实 tag 名；
- `test_version_file_plan_rejects_an_invalid_version`：VERSION 文件非法时报错。

### 2.5 `backend/internal/web/embed_on.go`（改上游文件，带锚点）

二开的**运行时前端覆盖**改造，让本地构建的前端产物直接映射进容器即可生效，不必重打镜像。

改动前上游的行为：`tryServeOverride` 只在**内嵌 dist 里已存在**该路径时才查 `data/public/`，且 `index.html` 固定走内嵌版。因此覆盖目录只能替换已有文件，投不了新文件、也改不了首页。

改动后：

| # | 位置 | 改动 | 原因 |
|---|---|---|---|
| 1 | `Middleware` | 先查覆盖目录，再判存在性；`index.html` 直接进 `serveIndexHTML` | 覆盖优先，且允许投放内嵌 dist 中不存在的新文件（否则会被当成 SPA 路由回落成 index.html） |
| 2 | 新增 `currentBaseHTML()` | 读 `data/public/index.html`，按 mtime+size 变化重新加载并失效已渲染缓存 | 首页可覆盖且改完即生效，不用重启容器 |
| 3 | `serveIndexHTML` / `injectSettings` | 用 `currentBaseHTML()` 代替 `s.baseHTML` | 让 1、2 两条生效 |
| 4 | `ServeEmbeddedFrontend`（legacy 分支） | 同上顺序调整 | 无 settings provider 时的回落路径保持一致 |

新增结构体字段：`embeddedHTML`（内嵌原始首页，用于回退）、`baseMu`（保护 baseHTML/overrideStamp）、`overrideStamp`。

**锚点清单**（合并上游后逐个确认还在、且只出现一次）：

| 锚点 | 位置 |
|---|---|
| `FORK-ANCHOR: override-first` | `Middleware` 里的覆盖优先分支 |
| `FORK-ANCHOR: override-index-loader` | `currentBaseHTML()` 定义 |
| `FORK-ANCHOR: override-index-use` | `serveIndexHTML` 里调用 `currentBaseHTML()` |
| `FORK-ANCHOR: override-first-legacy` | `ServeEmbeddedFrontend` 里的覆盖优先分支 |

### 2.6 `frontend/src/components/layout/AppSidebar.vue`（改上游文件）

四处改动，各带一个锚点：

**（1）屏蔽管理端侧栏三项入口**（`FORK-ANCHOR: sidebar-announcements-entry` / `sidebar-redeem-promo-entries`）——只删菜单项、不删路由，页面仍可直接输地址访问：

- `/admin/announcements`（公告）
- `/admin/redeem`（兑换码）
- `/admin/promo-codes`（优惠码）

随之删掉只为它们服务的 `BellIcon`、`TicketIcon` 图标定义（`noUnusedLocals` 会让未使用的 const 编译失败）（`FORK-ANCHOR: sidebar-removed-icons`）。`GiftIcon` 仍被用户端 `/redeem` 使用，保留。

**（2）新增「分组模型账号」侧栏入口**（`FORK-ANCHOR: sidebar-group-model-accounts`）——在 `adminNavItems` 的「账号管理」之后插入一条指向 `/admin/group-model-accounts` 的菜单项，图标用上游已有的 `FolderIcon`。

### 2.6b `frontend/src/router/index.ts`（改上游文件）

`FORK-ANCHOR: route-group-model-accounts` —— 在 `/admin/accounts` 之后注册 `/admin/group-model-accounts`，指向新增的 `views/admin/GroupModelAccountsView.vue`，`requiresAuth + requiresAdmin` 与其它管理路由一致。

### 2.6c `frontend/src/views/admin/GroupModelAccountsView.vue`（新增文件）

「分组模型账号」页面的正式实现，取代原来的运行时 DOM 注入面板。数据走既有管理 API（`adminAPI.groups.getAll()` + `adminAPI.accounts.list(page, 1000, { group })` 翻页），因此前端不用自建请求逻辑。

两种视角（模型→分组→账号、分组→模型→账号）、搜索、仅已调度过滤、刷新，行为对齐被它取代的注入面板。

**为什么不再用注入面板**：注入面板挂在 `/custom/group-model-accounts` 这个「自定义页面」路由上，而该路由的菜单项 `url` 必须填 `md:<slug>` 或绝对 http(s) 地址；站点对所有响应都发 `X-Frame-Options: DENY` + CSP `frame-ancestors 'none'`，绝对地址的 iframe 会被浏览器直接拒绝，`md:` 又只能渲染 Markdown。真路由页没有这些限制，且随镜像发布、升级后不用重建。

### 2.7 `backend/internal/web/embed_fork_test.go`（新增文件）

`FORK-ANCHOR: fork-embed-override-tests` —— 覆盖 2.5 的 fork 测试，与上游的 `embed_test.go` 分开放，减少合并冲突：

- `TestForkOverridePrecedesEmbeddedLookup`：内嵌 dist 中不存在的新路径能被覆盖目录命中（改动前会回落成 index.html）；
- `TestForkIndexHTMLOverrideAndCacheInvalidation`：`data/public/index.html` 生效、mtime 变化后重新加载、文件移除后回退内嵌版。

本机没有 Go，这两个用例的验证走 CI 的 `Fork embed override tests` 步骤。

### 2.8 账号管理搜索框支持按账号 ID 匹配（改上游文件，三处）

上游的账号列表搜索只匹配账号名（`account_repo.go` 里 `dbaccount.NameContainsFold(search)`），ID 是数字主键、名字里通常不含，因此按 ID 搜不到。

改动是**扩展搜索框的匹配范围**，不加新输入框、不改接口签名：

| # | 文件 | 位置 | 改动 | 锚点 |
|---|---|---|---|---|
| 1 | `backend/internal/repository/account_repo.go` | `accountListFilteredQuery` 的 search 分支 | search 为纯数字时改为 `Or(NameContainsFold(search), IDEQ(id))`，否则维持原样 | `account-search-by-id` |
| 2 | `frontend/src/views/admin/AccountsView.vue` | `accountMatchesCurrentFilters` | 本地过滤同步支持按 ID 匹配，避免编辑账号后该行被误判为不符合筛选而摘掉 | `account-search-by-id-local` |
| 3 | `frontend/src/i18n/locales/{zh,en}/admin/accounts.ts` | `searchAccounts` | 占位符改为「搜索账号名或 ID...」/「Search name or ID...」 | `search-placeholder-id-zh` / `search-placeholder-id-en` |

第 1 处是唯一有实际过滤作用的地方。`accountListFilteredQuery` 的三个调用点（账号列表、调度分候选池、数据导出）语义一致，改完都跟着生效；前端全选（`fetchAllAccountIds`）、导出、批量编辑的筛选快照都透传 `search`，不用逐个改。

### 2.9 账号管理搜索框 500ms 防抖与默认按账号 ID 降序（改上游文件，五处）

搜索框原来每输入一个字符就发一次列表请求；列表默认按名称升序，与「按 ID 定位账号」的用法不搭。这两点一起改。

| # | 文件 | 位置 | 改动 | 锚点 |
|---|---|---|---|---|
| 1 | `frontend/src/views/admin/AccountsView.vue` | 模板 `<AccountTableFilters>` | 搜索事件改接 `handleSearchQueryUpdate`，不再与下拉筛选共用 300ms 的 `debouncedReload` | `account-search-debounce` |
| 2 | `frontend/src/views/admin/AccountsView.vue` | `debouncedReload` 之后 | 新增 500ms 防抖 `debouncedSearchReload`：停顿后去首尾空格再刷新 | `account-search-debounce-handler` |
| 3 | `frontend/src/views/admin/AccountsView.vue` | 模板 `<DataTable>` | `default-sort-key` 改 `id`、`default-sort-order` 改 `desc` | `account-sort-default-id-desc` |
| 4 | `frontend/src/views/admin/AccountsView.vue` | `loadInitialAccountSortState` | 兜底值改 id/desc；新增一次性迁移键 `account-table-sort-default-version`，覆盖旧浏览器里存着的 name/asc | `account-sort-default-id-desc-fallback` |
| 5 | `frontend/src/components/admin/account/AccountTableFilters.vue` | `<SearchInput>` | 去掉 `@search` → `change` 的转发，搜索只保留一条触发路径 | `account-search-single-trigger` |

第 2 处的 `reload()` 是 AccountsView 自己的包装（重置分页并刷新今日统计），不是 `useTableLoader` 的 `reload`。第 4 处不覆盖的话，已访问过的浏览器会一直沿用旧的默认排序，改动等于没生效。首尾空格后端 `account_handler.go` 本来就有 `strings.TrimSpace`，第 2 处是让输入框内容与请求参数保持一致。按 ID 排序不需要改后端：`account_repo.go` 的 `accountListOrder` 早就支持 `sort_by=id`。

### 2.10 版本徽章：禁用在线更新、加 fork 仓库入口（改上游文件）

**背景**：`backend/internal/service/update_service.go` 的 `githubRepo` 硬编码为唯一上游，`POST /admin/system/update` 会下载**上游**二进制覆盖本实例。二开 fork 上这个入口是隐患，因此恒禁用；但「检查更新」保持指向上游，用来感知上游发版。

| # | 文件 | 改动 | 锚点 |
|---|---|---|---|
| 1 | `VersionBadge.vue` | 新增常量 `forkUpdateDisabled = true` 与 `FORK_REPO_URL` | `version-fork-config` |
| 2 | `VersionBadge.vue` | 「立即更新」按钮 `:disabled` 加 `|| forkUpdateDisabled`，并加 `title` 说明原因 | `version-disable-update` |
| 3 | `VersionBadge.vue` | 新增「查看 fork 仓库」链接。2026-10-05 挪到**全部状态分支之外**——原先只加在「有更新」（`hasUpdate && isReleaseBuild`）分支里，一旦上游没有新版本、界面落到最后的 `v-else` 分支，链接就不显示了 | `version-fork-repo-link` |
| 4 | `{zh,en}/misc.ts` | `version` 段新增 `viewForkRepo`、`updateDisabledByFork` | `version-fork-i18n-zh` / `version-fork-i18n-en` |

回归测试：`frontend/src/components/common/__tests__/VersionBadge.spec.ts`（二开新增文件）锁定「无更新」与「有更新」两种状态下「查看 fork 仓库」链接都在。

只禁用了 UI 按钮，**后端接口未动**——直接调 `POST /admin/system/update` 仍会执行上游覆盖。要彻底封死得改 `update_service.go`，目前按「不扩大改动面」保留。

### 2.11 侧栏收窄（改上游文件，三处）

展开态侧栏从 16rem（`w-64`，256px）收到 **13rem（`w-52`，208px）**，正好容纳 8 个中文字（最长菜单项「分组模型账号」6 字），余量还给右侧内容区。

| # | 文件 | 改动 | 锚点 |
|---|---|---|---|
| 1 | `AppSidebar.vue` | `:class` 里 `w-64` → `w-52` | `sidebar-compact-width` |
| 2 | `style.css` | `.sidebar` 的 `@apply w-64` → `w-52`（与 1 保持一致，否则两处宽度说法不一） | `sidebar-compact-width-css` |
| 3 | `AppLayout.vue` | 内容区 `lg:ml-64` → `lg:ml-52` | `layout-compact-offset` |

208px 的算法：内容区 `px-3`(24) + 链接内边距(31) + 图标(20) + 间距(12) = 87px 固定开销，剩 121px 给文字，8 个 14px 汉字需 112px。改 `w-48`（192px）只剩 105px，8 字会被截断。

### 2.12 国产供应商「API 协议」复选 + 兜底转发协议（改上游文件，新增 45 个锚点）

**背景**：原来 `credentials.api_protocol` 是**四选一**（adaptive / chat_completions / anthropic / responses）。
实际接入中会遇到「某站同时支持 OpenAI Chat Completions 与 Anthropic Messages，但**不支持** Responses」的站点，
单选无法表达「同时支持哪几个协议」，也没有「请求协议不在支持范围内时改用哪个协议转发」的表达能力。

**改动**：协议维度由单选改为**三协议复选** + 一个**兜底转发协议**（去掉 adaptive 选项）。三个协议仍是
chat_completions / anthropic / responses。OpenCode Go 平台**保持原单选 UI 不变**（它依赖 adaptive + 模型协议规则）。

#### 字段契约

| 字段 | 类型 | 含义 |
|---|---|---|
| `credentials.api_protocols` | `string[]` | 该站实际支持的上游协议（复选，非空、去重、只含平台支持的协议） |
| `credentials.fallback_protocol` | `string` | **兜底转发协议**：入站协议不在 `api_protocols` 内时改用该协议转发；必为 `api_protocols` 之一 |
| `credentials.api_base_urls` | `map[string]string` | 只包含已勾选协议的端点地址 |
| `credentials.base_url` | `string` | 勾选了 chat_completions 就用它的地址，否则用兜底协议的地址 |
| `credentials.api_protocol` | `string` | **兼容旧版的回退值**：勾选恰好 1 个协议且等于兜底协议时写该协议名，否则写 `adaptive` |

#### 路由语义

「勾选 = 声明该协议有原生端点，可零转换直通」：

| 入站请求 | 勾选集合含该协议 | 未勾选（走兜底） |
|---|---|---|
| `/v1/chat/completions` | 原生 CC 端点直通 | 按 `fallback_protocol` 转发（anthropic → 转 Anthropic；responses → CC→Responses 转换链） |
| `/v1/messages` | 原生 Anthropic 端点直通 | 同上 |
| `/v1/responses` | 原生 Responses 端点直通 | 同上（CC 兜底即转成 Chat Completions 再发） |

**向后兼容**：未配置 `api_protocols` 的旧账号完全走原 `api_protocol` 四选一逻辑，行为零变化
（`GetSelectedAPIProtocols()` 返回 nil 时所有新增分支都不接管）。

#### 后端改动（17 处锚点，全部在 `backend/internal/service/`）

| 文件 | 锚点 | 改动 |
|---|---|---|
| `account.go` | `fork-api-protocols-parse` | 新增解析层：GetSelectedAPIProtocols / HasExplicitAPIProtocols / GetFallbackAPIProtocol / SupportsAPIProtocol / ResolveAPIProtocolForInbound，兼容 []string / []any / 逗号串三种落库形态 |
| `account.go` | `fork-api-protocols-native-responses` | UsesNativeCNResponses 在复选模式下看 responses 是否在勾选集合内 |
| `account.go` | `fork-api-protocols-openai-base-url` / `-cn-base-url` / `-anthropic-base` / `-openai-format-base` | 端点地址解析：未勾选协议的端点请求回落到兜底协议地址；只勾 anthropic 时 OpenAI 格式端点回落平台默认 CC |
| `openai_gateway_forward.go` | `fork-api-protocols-responses-inbound` | /v1/responses 入站按勾选协议选端点 |
| `openai_gateway_forward.go` | `fork-api-protocols-raw-cc-gate` | shouldForwardOpenAIResponsesViaRawChatCompletions：responses 未勾选即转 CC 转发 |
| `openai_gateway_chat_completions.go` | `fork-api-protocols-chat-inbound` | /v1/chat/completions 入站分流 |
| `openai_gateway_messages.go` | `fork-api-protocols-messages-inbound` | /v1/messages 入站分流 |
| `openai_gateway_forward.go` / `openai_gateway_passthrough.go` / `openai_ws_forwarder_payload.go` | `fork-api-protocols-responses-base-forward` / `-passthrough` / `-ws` | 原生 Responses 出站取 responses 端点地址 |
| `openai_gateway_ollama_cloud_max_tokens.go` | `fork-api-protocols-ollama-responses-base` | 同上（ollama 路径） |
| `account_test_service.go` | `fork-api-protocols-test-routing` | 测试连接：复选账号走新探针 |
| `account_test_service_cn_adaptive.go` | `fork-api-protocols-test-selected` | 新增 testCNProviderSelectedProtocolsConnection：逐个验证已勾选的原生端点 |
| `upstream_billing_probe.go` | `fork-api-protocols-billing-probe-base` | 计费探测同样取 chat_completions 端点 |

新增文件 `backend/internal/service/fork_api_protocols_test.go`（带 `FORK:` 头，不算改上游）：6 个用例覆盖
解析、兜底回落、入站路由、端点地址解析与旧账号零回归。

#### 前端改动（28 处锚点）

| 文件 | 改动 |
|---|---|
| `credentialsBuilder.ts` | 新增 CN_API_PROTOCOLS / isCnNativeProtocol / normalizeCnProtocols / defaultFallbackProtocol / cnProtocolsFromLegacy / legacyProtocolFromSelection（字段契约的单一事实源） |
| `CreateAccountModal.vue` / `EditAccountModal.vue` | CN 平台：三协议**多选卡片** + **兜底转发协议下拉**（至少保留一个勾选，勾选变化自动联动兜底）；端点输入区只渲染已勾选协议；opencode_go 完整保留原单选 UI + 协议规则编辑器 |
| `{zh,en}/admin/accounts.ts` | 新增 protocolsHint / fallback / fallbackHint / selectAtLeastOne |
| 两个 spec | 期望改为新契约，新增「卡片 toggle 复选 + 兜底下拉联动」用例 |

#### 验证

- 后端：`go build ./...` 与 `go vet ./internal/service/` 通过；`go test -tags=unit -run TestForkProtocols ./internal/service/` 全绿。
  全量 `go test -tags=unit` 仅 `TestOllamaProbeCallback_StaleLongDoesNotOverrideNewShort` 失败——已用干净 HEAD 的
  git worktree 复现（8 次跑挂 5 次），确认是**上游既有的时间竞争 flake**，与本次改动无关。
- 前端：`vue-tsc --noEmit` 0 错误；`vitest run` 334 文件 / 2562 用例全绿。
- **注意**：本机 `NODE_ENV=production` 会让 vitest 里 VTU 的 stubs 全部失效（Vue 3.5 生产分支不接
  `transformVNodeArgs`），跑前端测试前必须设 NODE_ENV=test；这是环境问题，干净 HEAD 上同样全红。

### 2.13 端点配置区位置调整 + 一键「更新支持协议」（改上游文件，新增 29 个锚点）

**背景**（长空 2026-10-03 提出）：

1. 协议端点输入区原来在 API Key 区里，与「兜底转发协议」分处两处，改协议时要来回找；
2. 很多站点三个协议**共用同一个地址**，新勾选一个协议时端点被平台默认值预填，得手抄一遍；
3. 新接入一个站时不知道它支持哪些协议，做法是「先把三个都勾上 → 逐个测 → 手工取消测不通的」，
   缺一个「把测试结果一键写回账号」的按钮。

#### 改动 1：端点配置区移到「兜底转发协议」下方

- `CreateAccountModal.vue` / `EditAccountModal.vue`：`*-cn-endpoint-block` 从 API Key 区移到兜底协议下拉之后
  （DOM 顺序 = 视觉顺序），只渲染已勾选协议；API Key 区里只保留 opencode_go 自适应档的端点区（原行为）。
- 新勾选协议时默认**沿用上一个已勾选协议的地址**（`*-cn-protocol-endpoint-copy`）。规则三条：
  1. 目标端点已被用户改过（非空且 ≠ 平台默认值）→ 不动；
  2. 上一个已勾选协议的地址是用户自定义的 → 复制过来（自建站点最常见的形态）；
  3. 否则仅在目标端点为空时用上一个地址补空——官方平台三个协议地址本就不同，不能被覆盖。
- 新增文案 `endpointsHint`。

#### 改动 2：测试弹窗「更新支持协议」按钮（放在弹窗 footer 左侧）

- 只对国产供应商（kimi / zhipu / deepseek / minimax）显示；opencode_go 的协议是按模型规则推导的，不适用。
- 后端新增 `probeCNProviderProtocolsConnection`（新文件 `account_test_service_cn_protocol_matrix.go`，带 `FORK:` 头）：
  三个协议**各测一次**（用账号当前的 `api_base_urls` 端点），每个协议单独发 `protocol_probe` /
  `protocol_result` 事件；平台没有原生 Responses 端点（zhipu）时直接判定不支持、不发请求。
- 全部测完后按结果回写：通过者进 `api_protocols`，`fallback_protocol` 取 chat_completions > anthropic > responses
  的第一个，`base_url` 与 `api_protocol` 按既有契约同步（**全部失败则完全不改动账号配置**）。
- 后端为支撑"单个协议失败不是终止错误"加了两个开关：`accountTestSuppressErrorContextKey`
  （抑制 error 事件）与复用 `accountTestSuppressCompletionContextKey`（抑制内层探针的 test_complete，
  跑完矩阵前不让前端提前判定结束）。
- 协议结论用**指针字段** `protocol_ok` 上报：`success` 带 `omitempty`，false 会被整个省略，
  前端无法区分"失败"和"字段缺失"。
- 前端 `protocols-updated` 事件 → `AccountsView.vue` 重新拉账号详情并刷新列表。

#### 本轮锚点（29 个）

| 文件 | 锚点 |
|---|---|
| `backend/internal/service/account_test_service.go` | `-test-event-protocol` / `-test-sync-option` / `-test-matrix-routing` / `-test-suppress-error` |
| `backend/internal/handler/admin/account_handler.go` | `-test-sync-request` / `-test-sync-option-pass` |
| `frontend/.../AccountTestModal.vue` | `test-modal-sync-protocols-button` / `-state` / `-no-model-required` / `-start` / `-flag` / `-done` / `test-modal-protocol-events` / `-results` / `-results-reset` / `-protocols-updated-event` |
| `frontend/.../AccountsView.vue` | `test-modal-protocols-updated-refresh` / `-handler` |
| `frontend/.../CreateAccountModal.vue` | `create-cn-endpoint-block` / `create-cn-protocol-endpoint-copy` |
| `frontend/.../EditAccountModal.vue` | `edit-cn-endpoint-block` / `edit-cn-protocol-endpoint-copy` |
| `frontend/.../__tests__/*.spec.ts` | `test-create-cn-endpoint-block` / `test-edit-cn-endpoint-block` / `test-test-modal-sync-protocols` |
| `frontend/src/i18n/locales/{zh,en}/admin/accounts.ts` | `i18n-cn-endpoint-hint-{zh,en}` / `i18n-cn-sync-protocols-{zh,en}` |

新增测试文件 `backend/internal/service/fork_api_protocols_sync_test.go`（带 `FORK:` 头）：3 个用例覆盖
逐协议探测、按结果回写、全失败保持原配置。前端新增 4 个用例（端点区位置与复制语义 ×2、按钮与回写提示 ×2）。

**本轮踩坑**：`test_complete` 事件在函数返回前发出，而抑制开关是 `defer` 解除的，导致最终事件被自己吞掉
（表现为前端收不到完成事件、测试全失败）；已在发最终事件前显式解除两层抑制。`success` 字段的 `omitempty`
同样让"全失败"的完成事件看起来像成功，断言与前端判定都改为按 `protocol_ok` / 非 true 处理。`boolPtr` 与
本包 `ops_metrics_collector.go` 重名，新文件用 `forkBoolPtr`。`httpProtocolSyncUpstream` 的 `DoWithTLS`
签名写错（用了 `any` 而非 `*tlsfingerprint.Profile`），已复用既有的 `httpUpstreamRecorder`。

### 2.14 使用已有协议探测结果更新配置

- 「更新支持协议」只提交本轮连接测试收到的 `protocol_result`，不重新请求上游，成功后弹窗保持打开。
- 保存走 `PUT /api/v1/admin/accounts/:id/protocols`，不走通用账号更新入口，避免触发额外的 Responses 能力探测；服务端只接受国产供应商与有效协议名，兜底按 chat_completions > anthropic > responses 选取。
- 所有渠道的普通文本连接测试都支持编辑发送消息，默认值为：`我想使用你，你是什么模型呢？只回复我名字即可`；国产供应商协议矩阵的每条探测也尊重弹窗输入。
- 图像、视频、搜索、TTS 等专用测试继续使用各自的提示词；STT、Realtime 与 OpenAI Compact 不发送普通文本提示词。

### 2.15 本地验证与真实环境验证

本机没有线上账号数据，也无法访问真实上游；本地优先运行可行的单测和编译检查。若真实上游或线上数据是验收必要条件，不为此搭建临时模拟环境；经用户授权后直接推送到 fork，等待 GitHub Actions CI 通过，再部署到 NAS 做真实验证。本次实测结论以 CI 和 NAS 的实际结果为准。

### 2.16 openai 渠道「chat / responses」协议复选（改上游文件，新增 29 个锚点）

**背景**（长空 2026-10-04 提出）：openai 平台的中转站渠道与国产供应商一样存在「只支持 chat」「只支持
responses」「两个都支持」三种形态，但原来只有 `extra.openai_responses_mode` 三档下拉（自适应 / 强制
responses / 强制 chat），表达不了「两个都支持、按入站协议原样直通」。目标是把 2.12 的协议复选机制推广到
openai 平台的 API Key 账号，新增、编辑、测试三页 UI 与国产供应商一致，兜底协议保留。

**三种勾选组合的入站语义**（复用 2.12 的分流，不新写路由）：

| 勾选 | 入站 chat_completions | 入站 responses |
|---|---|---|
| 只 chat | 直连原生 CC | 转成 CC（兜底） |
| 只 responses | 转成 responses（兜底） | 直连原生 Responses |
| 两个都勾 | 直连原生 CC | 直连原生 Responses（零转换） |

**后端改动**（`backend/internal/service/`）

| 文件 | 锚点 | 改动 |
|---|---|---|
| `opencode_go.go` | `fork-api-protocols-openai-multiprotocol` | `IsMultiProtocolAPIKey()` 放开 openai：仅 API Key **且已写入 `api_protocols`/`fallback_protocol`** 时启用，未配置的存量账号走旧路径零变化；`IsMultiProtocolAPIKeyProvider(platform)` 保持不含 openai，分组级语义豁免不受影响 |
| `account.go` | `fork-api-protocols-openai-native-responses` | `SupportsNativeCNResponses()` 对 openai API Key 返回 true，否则 responses 会被勾选集合过滤掉 |
| `account.go` | `fork-api-protocols-openai-allowlist` | `GetSelectedAPIProtocols()` 对 openai 只放行 chat_completions / responses，anthropic 被剔除 |
| `account.go` | `fork-api-protocols-openai-base-url-default` | `defaultCNProtocolBaseURL()` 对 openai 回落账号自己的 `base_url` |
| `account.go` | `fork-api-protocols-cn-apikey-gate` | `GetCNAPIKey()` 收窄为 `IsCNProvider() \|\| IsOpenCodeGo()`，不让 openai 进 CN 余额 / 额度探测 |
| `openai_gateway_forward.go` | `fork-api-protocols-openai-raw-cc-gate` | `shouldForwardOpenAIResponsesViaRawChatCompletions()` 把 openai 复选账号并入 CN 分支 |
| `account_test_service.go` | `fork-api-protocols-openai-test-routing` | openai API Key 账号的**普通测试与「更新支持协议」都走两协议探测矩阵**，回写与否由 `testOpts.SyncProtocols` 决定 |
| `account_test_service_cn_protocol_matrix.go` | `fork-api-protocols-openai-sync-platform-gate` | `UpdateProbedCNProtocols` 平台校验放行 openai API Key |
| 新增 `account_test_service_openai_protocol_matrix.go` | 无（新文件带 `FORK:` 头） | 两协议探测矩阵、探测结论到 `extra.openai_responses_mode` 的映射、回写 |

**前端改动**：`CreateAccountModal.vue` / `EditAccountModal.vue` 对 openai API Key 渲染与国产供应商相同的
协议复选卡片 + 兜底下拉（不渲染分协议端点输入框，chat 与 responses 同域不同路径，端点共用账号
`base_url`）；旧的 `openai_responses_mode` 三档下拉隐藏，编辑页从该字段反推勾选以兼容存量账号；
`AccountTestModal.vue` 的「更新支持协议」按钮对 openai API Key 可见。

#### 2.16.1 首轮实测反馈的三个修复（长空 2026-10-04）

1. **普通测试不显示逐协议结果**。原来只有「已配置复选」或点「更新支持协议」才走矩阵，未配置的存量
   账号（公益站这类）直接走单协议测试，一个 `protocol_result` 事件都不发，测试弹窗里只有测试消息可改。
   改为与 DeepSeek 分组账号同语义：普通测试无条件跑两协议矩阵，逐协议上报结论与正文，但**不回写配置**。
2. **矩阵无条件回写**。`probeOpenAIAPIKeyProtocolsConnection` 收了 `syncProtocols` 参数却从未使用，
   普通测试也会改账号配置。补回 `if syncProtocols && len(passed) > 0`，并在非同步路径把 `accountRepo`
   换成 `protocolProbeAccountRepository`（no-op），避免探测过程把账号写成 error / ratelimited。
3. **「更新支持协议」按钮对 openai 永远不可点**。前端完成条件写死 `protocolResults.length !== 3`，
   openai 只有两个协议。新增 `expectedProtocolCount`（openai 2 / 其余 3）驱动该条件。

**代价**：openai API Key 账号每次点「测试」会打两次上游（chat + responses 各一次），这是拿到逐协议结果
的必然开销；未配置复选的存量账号生产转发行为不变。

**本轮锚点（1 个新增）**

| 文件 | 锚点 |
|---|---|
| `frontend/.../AccountTestModal.vue` | `test-modal-expected-protocol-count` |

**本轮踩坑**：`expectedProtocolCount` 的 `disabled` 断言要能红才算数——临时把条件回退成写死的 `3`
跑一次，确认这条前端用例失败（`attributes('disabled')` 返回 `''` 而非 `undefined`），再改回来。

### 2.17 账号列表「批量测试」（改上游文件，新增 12 个锚点）

**背景**（长空 2026-10-05 提出）：账号列表批量选中后只能逐个点「测试」，账号多时要开很多次弹窗。
目标是在批量操作栏加「批量测试」，一次跑完选中账号，页面按行显示各自的状态与逐协议结果。

**已定语义**（长空 2026-10-05 批准）

| 项 | 决定 |
|---|---|
| 支持范围 | 国产四家（kimi/zhipu/deepseek/minimax）+ openai API Key，与测试弹窗「更新支持协议」的判定一致 |
| 平台分组 | 按 `platform` 分组，各组各自取共有模型交集 |
| 模型来源 | 账号凭据里的 `credentials.model_mapping` 键，不请求上游 |
| 无 model_mapping | 排除出交集并给出原因，不回落去问上游 |
| 并发 | 3，不暴露成可调项 |
| 写回 | 测试只探测不写账号；另设「更新支持协议」按钮按账号逐个写回 |

支持范围以外的平台仍会出现在弹窗里，但标成不可测并给出原因（跨页全选时前端拿不到全部平台，
资格判定以后端返回为准）。

**后端**（新增文件 `account_batch_test_service.go`，新文件不带锚点）

- `BuildBatchTestPlan(ctx, accountIDs)`：按 platform 分组，逐账号判定资格（`IsCNProvider()` /
  `IsOpenAIApiKey()` 且 `model_mapping` 非空），组内取模型键交集。
- `RunAccountBatchTest(c, opts)`：带缓冲 channel 做并发闸门（默认 3），每账号一个
  `context.WithTimeout`（默认 90s）；用自定义 `http.ResponseWriter` 接住内层
  `TestAccountConnection` 的 SSE 输出，按事件边界切分后加上 `account_id` 转发出去，每个账号以
  `batch_test_complete` 收尾，前端据此定行状态。
- 执行形态复用 `RunTestBackground`（`account_test_service.go:3367`）那套
  `gin.CreateTestContext` + 自定义 writer，区别是要实时转发而不是跑完再解析。
- 探测期间不写库：openai / CN 矩阵路径在 `!syncProtocols` 时本就套了
  `protocolProbeAccountRepository`；批量测试也不调 `RecoverAccountAfterSuccessfulTest`。

**接口**

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/admin/accounts/batch-test-models` | 只读聚合：分组 + 共有模型 + 资格原因 |
| POST | `/api/v1/admin/accounts/batch-test` | SSE：每账号一组 `{account_id, model_id, prompt}` |

**前端**

新增 `AccountBatchTestModal.vue`（真组件，随镜像发布）：分组展示、每组一个模型下拉、逐行状态与
逐协议标签（未测灰 / 通过绿 / 失败红，打开弹窗就铺好占位）、测试正文默认展开；「更新支持协议」
按钮常显、无可写回行时置灰，按账号调已有的 `PUT /accounts/:id/probed-protocols` 写回，
不需要新的写回接口。`AccountBulkActionsBar.vue` 加按钮，`AccountsView.vue` 用 `selPlatforms`
判定显隐。

**本轮锚点（12 个新增）**

| 文件 | 锚点 |
|---|---|
| `backend/internal/handler/admin/account_handler.go` | `fork-batch-test-models-request` / `fork-batch-test-request` / `fork-batch-test-models-handler` / `fork-batch-test-handler` |
| `backend/internal/server/routes/admin.go` | `fork-batch-test-routes` |
| `frontend/src/api/admin/accounts.ts` | `batch-test-api` |
| `frontend/src/components/admin/account/AccountBulkActionsBar.vue` | `batch-test-button` |
| `frontend/src/views/admin/AccountsView.vue` | `batch-test-modal-state` / `batch-test-platform-gate` / `batch-test-open` |
| `frontend/src/i18n/locales/{zh,en}/admin/accounts.ts` | `i18n-batch-test-zh` / `i18n-batch-test-en` |

**测试**：`fork_batch_test_service_test.go`（8 单元）+ 前端 `AccountBatchTestModal.spec.ts`（7 用例）
+ `AccountBulkActionsBar.spec.ts` 加 1 用例。

**本轮踩坑**：测试里 stub upstream 要覆盖 `DoWithTLS` 而不是 `Do`——service 内部走的是前者。
只覆盖 `Do` 的话注入的延时不会生效，并发上限用例看着过、其实没有区分度（用例耗时显示 `0.00s`
就是信号）。

**2026-10-05 追加修正**（长空按线上截图提的三条，未新增锚点）

| 问题 | 处理 |
|---|---|
| 三个协议全红却标「通过」 | 协议矩阵（国产 / openai API Key）探测全失败时 `TestAccountConnection` 仍 `return nil`，批量执行器原先只看这个返回值。改为取 `test_complete` 的 `success` 判成败（`batchTestWriter` 在切分事件时记下结论）；跑完却没有 `test_complete` 也判失败，不给没结论的账号标通过 |
| 必须等整批跑完才能写回 | 「更新支持协议」测试进行中即可点击，只写「已跑完且已测通」的行（`probedRows` 从「有测通协议」收紧为「已收尾 + 有测通协议」）；写回前先把各行的通过协议快照下来，避免边跑边取拿到半截结果 |
| 想边测边调调度 | 每行加调度开关，直接调已有的 `POST /accounts/:id/schedulable`；候选接口的 `BatchTestAccountInfo` 增加 `schedulable` 字段 |

判成败的语义：**至少一个协议通过 = 通过**，与单账号弹窗「更新支持协议」一致；哪个协议没过由逐行
红色标记给出，收尾事件不再重复带错误文本。失败行仍可参与「更新支持协议」——只写它测通的那几个。

**2026-10-05 第三轮修正**（长空看线上弹窗提的三条，未新增锚点）

| 问题 | 处理 |
|---|---|
| 不到测完不知道要测几个协议 | 候选接口的 `BatchTestAccountInfo` 增加 `protocols`（`batchTestProtocols` 按平台给清单：国产 3 个、openai 2 个，与协议矩阵同序）。前端 `ProtocolResult.success` 放宽成 `boolean \| null`（`null` = 未测），打开弹窗就铺灰色占位标签，收到 `protocol_result` 按协议名找标签改色，清单外的协议才追加；新一轮测试把颜色退回灰、标签保留 |
| 「更新支持协议」测之前看不见 | 按钮去掉 `v-if` 常显，`:disabled="savingProtocols \|\| probedRows.length === 0"`，无可写回行时灰底，有结果后亮起 |
| 测试正文要手动点开 | `expanded` 初始值与每轮测试开始时都置 `true` |

协议标签三态配色：未测灰（`bg-gray-100`）、通过绿（`bg-green-100`）、失败红（`bg-red-100`）。

**2026-10-05 第四轮调整**（长空要求精简批量操作栏，新增 1 个锚点）

`AccountBulkActionsBar.vue` 里选中账号时不再渲染「批量启用调度 / 批量停止调度 / 探测上游倍率 /
批量刷新令牌」四个按钮，保留删除、重置状态、编辑、批量测试；锚点 `hide-legacy-bulk-actions` 标在
删除位置上。`defineEmits` 声明与 `AccountsView.vue` 侧的处理逻辑原样保留（只是没有按钮再触发），
`@probe-upstream-billing` 这些模板绑定的类型检查因此不受影响；恢复只需把按钮加回去。

**2026-10-08 第五轮：失败信息完整回传 + 逐协议换行**（长空，新增 11 个锚点）

起因是长空的三个要求：① 测试账号弹窗现在能同测三个协议了，**失败的信息也要返回，要完整、格式和之前一致**；
② 批量测试页面也要显示失败；③ 批量测试里一个账号的三个协议挤在同一行，**没有换行区分**。

| # | 问题 | 处理 |
|---|---|---|
| ① | 协议失败只显示「不可用」，看不见原因 | 后端在协议矩阵探测期间**收集**各协议推送到测试上下文的 `error` 事件文本（`accountTestErrorContextKey`），探测结束后随 `protocol_result` 的 `error` 字段发给前端；前端弹窗在失败协议卡片里加一行等宽小字显示完整报错，输出框同时按 `[协议] 失败原因：…` 追加一行，格式与既有的 `[协议]正文` 行一致 |
| ② | 批量测试只标红，看不到失败详情 | 逐协议报错存进 `ProtocolResult.error` 并按协议行显示；`batch_test_complete` 的 `error` 改为汇总各失败协议的 `协议名: 报错`（换行分隔），非协议级错误（超时、无完成事件）单独成行显示 |
| ③ | 一账号三协议同在一行 | 协议标签行保留（概览用），下方新增**逐协议行**：每协议一行，左边协议名+状态，右边返回正文或报错；容器用 `space-y-0.5` 纵向排布，失败正文与报错都 `whitespace-pre-wrap break-all` |

**为什么要在后端收集报错**：协议矩阵用 `accountTestSuppressErrorContextKey` 抑制了各协议的
`error` 事件（单个协议失败是「该协议不支持」的正常结论，不是整场测试的终止错误），抑制之后报错文本
就丢了，前端手里只剩一个 `protocol_ok: false`。收集动作与既有的 `content` 收集一样**放在抑制判断
之前**（`sendEvent` 里），所以抑制策略不变、只是把文本留了下来。

**报错文本的优先级**：协议矩阵优先用收集到的报错文本覆盖探测函数的返回值文本——返回值常是
`API returned 400` 这种概括，收集到的才是带上游原文的完整信息。两者都没有才回落「不可用」。

**本轮锚点（11 个新增）**

| 文件 | 锚点 |
|---|---|
| `backend/internal/service/account_test_service.go` | `protocol-matrix-error-collector` / `protocol-matrix-error-collect` |
| `.../account_test_service_cn_protocol_matrix.go` | `protocol-matrix-collect-error-text` / `protocol-matrix-error-event` / `protocol-matrix-collect-error-text-seq` / `protocol-matrix-error-event-seq` |
| `.../account_test_service_openai_protocol_matrix.go` | `protocol-matrix-collect-error-text-openai` / `protocol-matrix-error-event-openai` |
| `.../account_batch_test_service.go` | `batch-test-protocol-failures` / `batch-test-protocol-failure-text` / `batch-test-complete-failures` |
| `frontend/.../AccountTestModal.vue` | `test-modal-protocol-error-field` / `test-modal-protocol-error-view` / `test-modal-protocol-error-capture` / `test-modal-protocol-error-line` |
| `frontend/.../AccountBatchTestModal.vue` | `batch-test-protocol-error-field` / `batch-test-protocol-body-field` / `batch-test-row-failure-field` / `batch-test-protocol-lines` / `batch-test-row-failure-line` / `batch-test-protocol-line-helpers` / `batch-test-failed-with-body` / `batch-test-protocol-error-capture` / `batch-test-protocol-body-capture` / `batch-test-error-append` / `batch-test-complete-failure-capture` / `batch-test-append-failure` |
| `frontend/src/i18n/locales/{zh,en}/admin/accounts.ts` | `i18n-protocol-probe-error-{zh,en}` / `i18n-batch-test-failure-{zh,en}` |

**测试**：Go `fork_batch_test_service_test.go` 改写 `TestForkBatchTestRunCompletesFailedTarget`
（收尾事件现在**要**带失败汇总）+ 新增 `TestForkProtocolMatrixReportsFailureError`（三协议各自的
完整报错 + 汇总）；前端 `AccountTestModal.spec.ts` +1 用例、`AccountBatchTestModal.spec.ts` +3 用例。

**本轮踩坑**

1. **失败行被 `!row.expanded` 挡住**。第一批实现把行级失败原因写成 `v-if="... && !row.expanded"`，
   而行的默认就是展开的，结果失败文本一次都没渲染出来（`batch-row-2` 的用例报
   `expected ... to contain 'boom'`）。行级失败原因要**无条件可见**，它承载的是逐协议报错之外
   的收尾错误，用户不该为了看它先点「收起」。
2. **测试桩里的 JSON 内层引号没转义**。`"error":"401 {"error":{...}}"` 会让整个 SSE 行解析失败、
   该事件被静默丢弃，现象是「只有部分协议有结果」。写协议报错桩时要么转义内层引号，要么用不含
   引号的文本。
3. **新增用例要做变异验证**。把逐协议行容器的 `v-if` 临时改成 `false` 跑一遍，确认 3 个新用例
   变红（`3 failed | 7 passed`），证明用例真的在验行为、不是恒过。
4. **CN 协议矩阵只在 `赛博羊毛-DS` 分组账号上并发探测**（`isDeepseekDSGroupAccount`）。写
   「三协议报错」的用例时账号必须挂该分组，否则退回单协议路径，只拿到一份结果。
5. **改完必须跑真正的 vue-tsc**。第一版把 `BatchRow` 的 `failure` 改名 `rowFailure` 时，误把
   `excerpt: string` 的声明整行替换掉了，模板与事件处理里仍有 5 处引用 `row.excerpt`，CI 的
   `build-frontend` 直接失败（TS2339/TS2353）。原因是本机 `pnpm run typecheck` 会先去解析依赖并
   报 `ERR_PNPM_IGNORED_BUILDS`，根本跑不到 tsc 就退出了，而我只跑了 vitest（vitest 不做全量类型
   检查，这类错误它抓不到）。**正确做法**：直接用本地二进制
   `node_modules/.bin/vue-tsc.cmd --noEmit -p tsconfig.json`，绕开 pnpm 的依赖解析。

**2026-10-08 第六轮：测试弹窗默认显示协议占位**（长空要求：默认就显示「协议探测结果」，
三个协议是灰色的，点测试之后再变色）

单账号测试弹窗原来要等第一个 `protocol_result` 事件才出现面板（`v-if="protocolResults.length > 0"`），
点测试前完全看不到会测哪几个协议。改为：

- 打开弹窗就按平台铺好卡片：国产三家 `chat_completions / anthropic / responses`，openai API Key 只有
  `chat_completions / responses`，顺序对齐后端 `cnProtocolProbeOrder` / `openaiAPIKeyProtocolProbeOrder`。
- `ProtocolProbeResult.success` 由 `boolean` 放宽为 `boolean | null`（`null` = 未测），三态配色：
  未测灰（`bg-gray-50`）+ `clock` 图标 + 「未测试」/ 通过绿 / 失败红。
- 事件到达时就地改那一张卡片的颜色，不再 push 新条目（清单外的协议才追加，兜底）。
- 重跑测试时退回灰色占位（`resetState` 里 `protocolResults = buildProtocolPlaceholders()`）。
- 不跑协议矩阵的平台（如 grok）不显示该面板（`hasProtocolPanel`）。

**这一轮踩到的坑**：同步按钮原判据是 `protocolResults.length !== expectedProtocolCount`，
占位卡片一开着弹窗就在了，会让按钮**没测试就可点**。改为 `testedProtocolCount`（数 `success !== null`
的数量）。这是「占位」与「结果」共用同一个数组带来的必然副作用，改这类状态时要注意所有依赖
`length` 的判断。

锚点：`test-modal-protocol-placeholders` / `test-modal-tested-protocol-count` /
`test-modal-protocol-three-state` / `test-modal-protocol-seed-on-open` /
`i18n-protocol-probe-pending-{zh,en}`（`test-modal-protocol-results-reset` 语义改为「退回占位」）。

测试：`AccountTestModal.spec.ts` 新增 3 个用例（占位三态与改色、openai 只铺两张、非矩阵平台不显示），
共 12 个；并做变异验证——把占位构造改成空数组时 2 个用例如期变红。

**2026-10-08 部署记录**：镜像 `ghcr.io/hjkl950217/sub2api:0.2.14`（manifest list digest
`sha256:a968d3d980fe0a0b98f514dfd8ca431ae04867d4985a0d7e96f6f776fb54819f`），`v0.2.14` 已指向
`848f9f38f`。NAS 运行 revision `848f9f38f06afafd1c069664479955a46f856417`，`healthy`，入口 chunk
由 `index-D2EC3ehM.js` 变为 `index-CPNHPSPA.js`，测试弹窗所在 chunk
`AccountsView-C8n5FA70.js` 回读含 `protocol-error-` / `protocolProbeErrorLabel` /
`batch-protocol-line-` / `batch-failure-` 四个新标记。

**NAS 拉取镜像踩坑（与本次代码无关，但会挡住后续所有部署）**：NAS 的 Docker 守护进程在
`/etc/docker/daemon.json` 里配了 `proxies.https-proxy = localhost:21089`（→ `clash4docker` 容器的
7890），而该 Clash 的机场节点已失效（日志 `dial tcp 18.140.71.40:443: i/o timeout`，经代理请求返回
502 / `SSL_ERROR_SYSCALL`）。于是 daemon 拉任何镜像都报 `Get "https://ghcr.io/v2/": EOF`，而**直连
ghcr.io 是通的**（token 端点 200、`/v2/` 401）。处理：把 `ghcr.io,*.githubusercontent.com` 追加到
`proxies.no-proxy`（原文件已备份为 `/etc/docker/daemon.json.bak.20261008`），`systemctl reload docker`
后 `docker pull` 正常。**注意 `ctr -n moby images pull` 虽然能绕过 daemon 代理拉到镜像，但 Docker 用的
是自己的 overlay2 存储（`/var/lib/docker`），与 containerd 的存储不互通，`docker images` 里看不到，
不能用来交付镜像。** GCHR 包对该仓库允许匿名拉取 token，不需要 `docker login`。

### 2.18 使用记录页：时间列可配置 + 贴底横向滚动条（改上游文件，新增 8 个锚点）

**背景**（长空 2026-10-05 提出）：管理端 `/admin/usage` 的表格列多，横向滚动条压在表格最底部，
要先滚到页面底部才够得着；同页的时间列原先也没法在「列设置」里开关。

**时间列交给列设置**

用户端 `/usage` 与管理端 `/admin/usage` 的用量明细、错误请求四张表，`created_at` 原先写死在
`ALWAYS_VISIBLE` 里强制显示，列设置下拉里根本没有这一项。四处都把它移出必显名单：默认仍然显示
（`DEFAULT_HIDDEN_COLUMNS` 不含它），列设置里多出「时间」可以自行关掉。管理端的「用户」列、
错误请求的「状态码」「操作」列仍保持必显。

| 文件 | 锚点 |
|---|---|
| `frontend/src/views/user/UsageView.vue` | `usage-time-column-toggle-user` / `usage-time-column-toggle-user-errors` |
| `frontend/src/views/admin/UsageView.vue` | `usage-time-column-toggle-admin` / `usage-time-column-toggle-admin-errors` |

**贴底横向滚动条**

新增 `frontend/src/components/common/FloatingHorizontalScrollbar.vue`：横向滚动的真身是 DataTable
内部的 `.table-wrapper`，组件接收外层容器、在里面找它，在页面末尾放一条 `sticky bottom-0` 的横条。

滑块是**自绘**的。起初直接让横条 `overflow-x: auto` 用原生滚动条，线上根本看不见：`style.css:42`
全局把 `::-webkit-scrollbar` 定成 `h-2`（8px 高）、`::-webkit-scrollbar-thumb` 默认 `bg-transparent`
（只有 `*:hover::-webkit-scrollbar-thumb` 才半透明显形），而 Edge 的 overlay 滚动条又不吃
`::-webkit-scrollbar` 的高度定制——DataTable 第 1101 行那段 `scrollbar-width: auto !important`
注释踩的是同一个坑。现在 track 里放一个宽度按 `clientWidth / scrollWidth` 比例算的圆角滑块
（下限 48px，太短不好抓），拖它按比例改真容器的 `scrollLeft`，真容器滚动时滑块跟着走；内容没溢出
时整条 `v-show` 收掉，`ResizeObserver` 盯容器、table 和 track 的宽度变化。

接入点在 `views/admin/UsageView.vue`（锚点 `floating-h-scrollbar`）：两个 tab 容器各加 ref，
`activeTableContainer` 跟着 `activeTab` 走。浮动条必须放在两个 tab 容器的**外面**——那两个容器带
`overflow-hidden`，在里面 `sticky` 会失效。

**两个页面都接了**：管理端 `/admin/usage` 的两个 tab 用 `v-show`；用户端 `/usage` 的两个 tab 用
`v-if`，把原来的 `<template>` 拆成带 ref 的 `<div>` 才挂得上。

**位置与形态**（长空第二轮反馈后定）

条放在两个 tab 容器**内部、分页之前**，容器原来的 `overflow-hidden` 要去掉——留着的话 `sticky`
在里面失效，滚到底不会停在分页上方。滚动过程中它粘在视口底部（浮动），滚到页面底部时停在自然
位置，也就是分页上方。错误请求 tab 的分页在 `OpsErrorLogTable` / `UserErrorRequestsTable` 内部，
条只能跟在这两个组件后面。track 24px 高，滑块 12px、hover 到 16px；没有可滑内容时滑块铺满整条、
整条降到 `opacity-70`、光标不是抓手。

**测试**：`FloatingHorizontalScrollbar.spec.ts`（2 用例：溢出时显示并同步 `scrollLeft`、不溢出时隐藏）
+ `admin/UsageView.spec.ts` 新增 2 用例（时间列默认进表头、且出现在列设置下拉里）。

### 2.19 reasoning 改走标准 Responses reasoning 输入项（改上游文件，新增 3 个锚点）

**背景**：站点侧思考块泄露——见 `文档/过程记录/报告_20261005_思考块泄露与同分组调度调研.md` 的链 A。
`chatcompletions_to_responses.go` 的 `chatAssistantToResponses` 把 assistant 历史的
`reasoning_content` 包成 `<thinking>…</thinking>` 拼进发往上游的 `output_text`，上游模型会模仿
这个格式并把它回显到正文。2026-09-28 的实测：有包装 12/30 泄露，无包装 0/30。

**改法**：产出标准的 reasoning 输入项

```json
{"type":"reasoning","summary":[{"type":"summary_text","text":"…"}]}
```

与读取侧往返对称——`chatcompletions_responses_bridge.go` 的 `buildChatMessagesFromItems` 本来就是
从 `summary` 里取回 `reasoning_content`，写入侧产出同形状，格式闭环。
`chatcompletions_responses_request_invariants_test.go:47-52` 的 golden 样本正是这个形态
（reasoning item 后面直接跟 function_call），新代码产出的形状与它一致。

**不能直接删**：`chatcompletions_responses_bridge.go:404-417` 与 `:636-642` 的注释写明 DeepSeek
thinking mode 要求 assistant 消息回传 `reasoning_content`，丢了会报 400。所以是**换承载形式**，不是去掉。
另：上游要求 reasoning 项必须带 `summary`，缺失报 400 `Missing required parameter 'input[N].summary'`
（`openai_codex_transform.go:1622-1626`）；该层逐字段保留 `summary`，只在缺失时补空数组。

| 文件 | 锚点 |
|---|---|
| `backend/internal/pkg/apicompat/types.go` | `reasoning-input-summary` |
| `backend/internal/pkg/apicompat/chatcompletions_to_responses.go` | `reasoning-input-item` / `assistant-content-split` |

**副作用**：assistant 消息只有 reasoning、没有正文时，不再产出一条承载 `<thinking>` 的空 assistant
message item，输入项序列因此变长。`parseAssistantContent` 的返回值从 `(string, error)` 改成
`(text, reasoning string, err error)`。

**测试**：`chatcompletions_responses_test.go` 3 个用例改写（2 个改名成
`..._AssistantThinkingBecomesReasoningItem` 与 `..._AssistantReasoningBecomesReasoningItem`）；
`openai_gateway_grok_chat_bridge_test.go:484` 的断言改为验证 `input.1.type=reasoning` 与
`input.1.summary.0.text`。

### 2.20 编辑弹窗查看 API Key 明文（改上游文件，新增 14 个锚点）

**背景**：编辑账号弹窗的 API Key 输入框是空的密码框，只能靠「留空 = 不修改」判断，想核对当前
填的是哪把 key 得去翻数据库。长空 2026-10-12 要求：默认显示 `****`，右侧加眼睛按钮点开看明文。

**为什么必须有后端配合**：上游 `GET /admin/accounts/:id` 的响应经 `dto.RedactCredentials`
（`credentials_redact.go`）剥掉 `service.SensitiveCredentialKeys` 里的子键，只留一个
`credentials_status.has_api_key` 布尔值，前端根本拿不到 `api_key` 原文。

**改法**：

| 层 | 文件 | 锚点 |
|---|---|---|
| 接口 | `backend/internal/handler/admin/account_handler.go` | `account-reveal-api-key-handler` |
| 路由 | `backend/internal/server/routes/admin.go` | `account-reveal-api-key-route` |
| 前端 API | `frontend/src/api/admin/accounts.ts` | `account-reveal-api-key-api` / `account-reveal-api-key-api-export` |
| 输入框 | `frontend/src/components/account/EditAccountModal.vue` | `edit-api-key-reveal-input` / `edit-api-key-reveal-input-upstream` / `edit-api-key-reveal-state` / `edit-api-key-reveal-reset` / `edit-api-key-reveal-submit` / `edit-api-key-reveal-submit-upstream` |
| 文案 | `frontend/src/i18n/locales/{zh,en}/admin/accounts.ts` | `edit-api-key-reveal-i18n`（两处同名） |
| 用例 | `backend/internal/handler/admin/fork_account_reveal_api_key_test.go`<br>`frontend/src/components/account/__tests__/EditAccountModal.apiKeyReveal.spec.ts` | `account-reveal-api-key-test`<br>`test-edit-api-key-reveal` |

**接口语义**：`GET /api/v1/admin/accounts/:id/credentials/reveal` → `{"api_key":"sk-…"}`。
只回 `api_key` 一个键，token / 私钥 / `service_account_json` 等其他敏感凭证不外带；挂在管理端
路由组，走已有的管理员鉴权，不额外加 step-up（与允许改凭据的 `PUT /accounts/:id` 同级）。
现有 `GET /accounts/:id` 的脱敏逻辑一行未动。

**前端语义**：

- 打开弹窗时若 `credentials_status.has_api_key` 为真，**立即**调一次 reveal（同步阶段先显示 `****`
  占位），拿到明文长度后把框里画成**等长的星号掩码**——长空的 key 约 50 字符，就画约 50 个 `*`；
  取不到就退化成 `****`。没有 key 的账号框里为空。
- **眼睛按钮只负责显隐，不再发请求**：明文在打开弹窗时已经拿到并缓存在 `apiKeyPlain`。
- 掩码态点进输入框会自动清空，方便直接粘新 key（避免出现 `***sk-new`）。
- 提交时星号掩码与空串都当作「不修改」，不会把掩码写进 credentials。
- 通用 API Key 字段与 upstream 类型的 API Key 字段共用同一套状态，都带眼睛。

**测试**：`vue-tsc --noEmit` 退出码 0；`vitest run src/components/account/__tests__/`
32 文件 508 用例全过（含本轮新增 6 条）。本机无 Go 工具链，后端用例由 push 后的 CI 跑。

### 2.21 DeepSeek 主机上的 openai 账号强制走 chat（改上游文件，新增 2 个锚点）

**背景**（上游 issue #7285）：`platform=openai` + `base_url=https://api.deepseek.com` 的 API Key 账号，
创建时被 `ProbeOpenAIAPIKeyResponsesSupport` 标记 `extra.openai_responses_supported=true`，
之后 Codex（Responses Lite）的工具调用走原生 `/responses`——DeepSeek 端点返回 200 却**静默忽略**
`input[].additional_tools` 里的工具声明，模型于是把调用当正文吐成 `<||DSML||tool_calls>…` 文本，工具不执行。

探测判定不出这个差异：探测请求用的是**顶层 `tools`**，DeepSeek 认得；而 Codex 用的是 `input[].additional_tools`，
DeepSeek 不认。两者形态不同，HTTP 层看不出区别，所以探测会误判为「支持」。

**改法**：在探测函数里按**主机名**判定，`base_url` 主机为 `api.deepseek.com` 时直接落标
`responses_supported=false` + `responses_mode=auto`，跳过网络探测，让网关走 `/v1/chat/completions`
（DeepSeek 的 chat 路径本就有工具与 reasoning 适配）。只认官方主机名，避免误伤自建中转。

| 文件 | 锚点 |
|---|---|
| `backend/internal/service/openai_apikey_responses_probe.go` | `probe-deepseek-host-check`（判定函数）/ `probe-deepseek-host-forces-chat`（探测落标分支） |

新增测试文件 `backend/internal/service/fork_deepseek_responses_probe_test.go`（带 `FORK:` 头，不算改上游）：
覆盖主机名判定（含后缀伪装、端口、大小写）、DeepSeek 主机不打上游且落标 false、非 DeepSeek 主机仍照常探测。

**已知边界**：探测只在账号创建/更新时跑（`scheduleOpenAIResponsesProbe`），**存量账号**的
`openai_responses_supported=true` 不会自动纠正，需重新保存账号一次才落标。若要覆盖存量账号，
需在转发路径再加一处 host 判定（本轮未做）。

### 2.22 仪表盘「图表设置」：统计图表可开关（改上游文件，新增 11 个锚点）

**需求**（长空 2026-10-14）：仪表盘 3 张统计图表加「图表设置」，用开关控制显隐；关掉的图表不渲染、
也不请求对应统计接口；默认隐藏「Token 使用趋势」和「最近使用」。

**改法**：

- 图表清单固定 3 项（模型分布 / Token 使用趋势 / 最近使用），顺序即渲染顺序，不做拖拽排序。
- 入口在「时间范围」卡片里、刷新按钮右侧，点开 `BaseDialog`；列表按行分割（`divide-y`），
  开关在名字左边、名字下面是描述，不用表格。
- 按需请求：`getSnapshotV2` 的 `include_trend` / `include_model_stats` 跟随开关；
  「最近使用」关掉就不调 `getUserUsageTrend`，「模型分布」关掉就不调 `getUserSpendingRanking`。
- 布局：`grid-cols-1 lg:grid-cols-2`。可见的半栏图表只剩一个时加 `lg:col-span-2` 撑满整行，
  「最近使用」恒占整行。
- 开关状态存 `localStorage['dashboard-visible-charts']`，**每浏览器**记忆，不跟账号同步。
  存储格式后来改成了 `{id: 布尔}` 映射（见 2.23），这条当时是 id 数组。

| 文件 | 锚点 |
|---|---|
| `frontend/src/views/admin/DashboardView.vue` | `dashboard-chart-settings-state`（图表清单与可见性）/ `dashboard-chart-settings-storage`（本地存储读写）/ `dashboard-chart-settings-toggle`（切换开关并重拉数据）/ `dashboard-chart-settings-requests`（按开关决定请求哪些接口）/ `dashboard-chart-settings-grid`（栅格与条件渲染）/ `dashboard-chart-settings-button`（入口按钮）/ `dashboard-chart-settings-dialog`（设置弹窗）/ `dashboard-chart-settings-recent-usage`（最近使用卡片） |
| `frontend/src/i18n/locales/zh/admin/overview.ts`、`frontend/src/i18n/locales/en/admin/overview.ts` | `i18n-dashboard-chart-settings-zh` / `i18n-dashboard-chart-settings-en` |
| `frontend/src/views/admin/__tests__/DashboardView.spec.ts` | `test-dashboard-chart-settings` |

上游用例 `switches metrics and ignores a stale tokens response` 跟着改了：它原本默认「最近使用」可见，
现在要先在设置弹窗里把图表打开。同时给外层 `beforeEach` 补了 `localStorage.clear()`，避免开关跨用例串味。

**已知边界**：`localStorage` 每浏览器一份，换浏览器或清缓存会回到默认（只开「模型分布」）。

### 2.23 仪表盘区间统计卡片 + Token 使用趋势改堆叠柱（改上游文件，新增 7 个锚点）

**背景**（长空 2026-10-14）：仪表盘顶部那 8 张统计卡片来自 `GetDashboardStats(ctx)`，**不带日期参数**——
`total_*` 是累计、`today_*` 是今日，所以「所选时间范围内一共用了多少 token / 缓存率多少 / 请求多少次」
拿不到。另外原来的 Token 使用趋势是四条填充折线加一根右轴百分比线，两条轴互相压着，看不清楚。

**改法**：

- 新增两张卡片，都进「图表设置」列表、可开关：
  - **区间用量概览**（整行）：总 Token / 请求次数 / 缓存率 / 实际花费。副标题依次是日均 Token、
    日均请求、缓存省下的输入量、标准价。数字样式照抄顶部统计卡片（彩色图标 + 小标题 + 大数字 +
    小字副标题），装在同一个框里。
  - **Token 构成占比**（半栏）：输入 / 输出 / 缓存写 / 缓存读的横向堆叠条，下面跟一份按行分割的图例。
- **不加后端接口**：区间合计由 `trend` 求和得到。`usage_log_repo_trend.go:544` 里
  `total_tokens = input + output + cache_creation + cache_read`，所以四段能凑满 100%。
- 缓存率口径与 `TokenUsageTrend.vue` 一致：`cache_read / (input + cache_read + cache_creation)`。
- **Token 使用趋势**改成堆叠柱：新建 `TokenUsageStackedBar.vue` 只给仪表盘用，共享的
  `TokenUsageTrend.vue` 原样保留（另外 3 个页面还在用折线）。堆叠柱去掉了右轴百分比线。
- 排版规则改成：可见的半栏图表为**奇数**个时，最后一个撑满整行（原来是「只剩 1 个时撑满」）。
- 开关存储改成 `{id: 布尔}` 映射，**没记录的走默认值**——以后再加图表时，早就存过设置的人也能
  看到它，而不是被判成「关」。

| 文件 | 锚点 |
|---|---|
| `frontend/src/views/admin/DashboardView.vue` | `dashboard-range-cards`（两张新卡片）/ `dashboard-range-days`（区间概览的日均分母）/ `dashboard-token-usage-stacked-bar`（折线换柱） |
| `frontend/src/i18n/locales/zh/admin/overview.ts`、`frontend/src/i18n/locales/en/admin/overview.ts` | `i18n-dashboard-range-summary-zh` / `-en`、`i18n-dashboard-token-composition-zh` / `-en` |

新增文件（带 `FORK:` 头，不算改上游）：`components/charts/RangeUsageSummary.vue`、
`TokenCompositionBar.vue`、`TokenUsageStackedBar.vue`、`rangeTotals.ts`（求和 / 缓存率 / 格式化），各配一个 spec。

**已知边界**：

- 区间天数按粒度算：按小时粒度（区间 ≤1 天）记作 1 天，此时不显示日均——否则日均和总量是同一个数。
- 「区间用量概览」和「Token 构成占比」都依赖 `trend`，所以这两张和趋势图只要有一张开着，
  `include_trend` 就是 true。

### 2.24 仪表盘卡片调整 + 全站数字改中文习惯（改上游文件，新增 2 个锚点）

**背景**（长空 2026-10-14，看完 2.23 的实测反馈）：

1. 区间用量卡片不用重复显示所选时间范围（上方「时间范围」筛选器已经有了）
2. 「Token 构成占比」改名「Token 构成」；图例按占比降序，一般「缓存读」排第一
3. 模型分布排到 Token 构成前面
4. 图表图例写死了英文（Input / Output / Cache Creation / Cache Read / Cache Hit Rate），改中文
5. **全站数字改成中文习惯写法**

**改法**：

- 区间用量卡片去掉右上角的时间范围，连带删掉 `DashboardView` 里的 `rangeLabel` computed
- `TokenCompositionBar` 的桶按 token 数降序排（`Array.sort` 是稳定排序，数值并列时保持清单顺序）
- `DASHBOARD_CHART_IDS` 顺序改为 rangeSummary → modelDistribution → tokenComposition →
  tokenUsageTrend → recentUsage，「图表设置」列表顺序同步变化
- `TokenUsageTrend.vue` / `TokenUsageStackedBar.vue` 的图例改走 i18n，复用
  `admin.dashboard.tokenComposition.*`，命中率用新增的 `admin.dashboard.cacheHitRate`

**数字中文写法**（实现集中在 `utils/format.ts`，全站共用）：

| 函数 | 用途 | 规则 |
|---|---|---|
| `formatNumber` | 计数（token、请求数、用户数…） | 1 万以下直接给数字；1 万～1 亿折成 `X.XX万`；过亿折成 `X.XX亿`，固定两位小数 |
| `formatCost` | 金额（新增，不带货币符号） | 整数部分每 4 位一个逗号；小数位沿用原档位（≥1 两位、≥0.01 三位、更小四位） |
| `formatCurrency` | 金额（带货币符号） | 用 `formatToParts` 关掉 Intl 自带分组，再按 4 位重组 |
| `formatCompactNumber` | 旧名 | 改成 `formatNumber` 的别名；`allowBillions` 参数已去掉 |

- **不再走 Intl 的 locale 分支**：界面切英文时数字也保持同一套写法，结果可预期
- 样例：`1000→1000`、`12345→1.23万`、`12345678→1234.57万`、`123456789→1.23亿`；
  `99999999→1.00亿`（万位会进位到 10000.00 万，这种写法直接进位成亿）
- 原来散在 12 个组件里的本地 `formatTokens`、13 个本地 `formatCost` 全部删掉，改从
  `utils/format` 引；顺手把 `UsageTable` / `StatCard` / `KeyUsageView` / `AffiliateView` 等处的
  裸 `toLocaleString()` 计数一并换掉。**日期类的 `toLocaleString` 不在此列**，保持原样

| 文件 | 锚点 |
|---|---|
| `frontend/src/utils/format.ts` | `chinese-number-grouping`（4 位分组 + 万/亿 的实现） |
| `frontend/src/components/charts/TokenUsageTrend.vue` | `token-usage-trend-cn-labels`（图例改中文） |

**风险**：这是全站显示口径的改动，涉及 30 多个文件。用量明细表（`UsageTable`）每行的 token
也从精确值变成了 `1.23万` 这种缩写——如果你看明细时更需要精确位数，单独回退那个文件即可。

### 2.25 账号测试弹窗「模型指纹测试」（改上游文件，新增 11 个锚点）

集成 [lm-detector](https://github.com/Ikaleio/lm-detector)（CLI 名 `fpd`，npm 包 `lmfpd`，MIT）
的模型指纹检测，用来判断某个中转站是否真在提供它声称的模型。
**本次只做前端 + mock，后端还没接。**

判定原理：让模型凭第一反应写约 300 个 1–355 的整数，比对各模型稳定的数字分布偏好，
在参考库（64 个指纹身份）里给出最接近的候选。**是闭集排序，不是身份证明**，UI 上必须带这句免责。

#### 前端（本次）

- 新增 `frontend/src/components/admin/account/fingerprintMock.ts`（**新增文件，只带 `FORK:` 头**）：
  类型定义 + 一份真实检测报告示例数据 + `runFingerprintMock()`（固定 3 秒延迟）。
  示例数据取自一次真实检测（账号 3285「云枢」/ `https://yunshu.best` / 请求模型 `claude-opus-5-5`），
  完整原始返回见 `临时文件/lm-detector-sample-output.json`。
  **接后端时只换 `runFingerprintMock` 的函数体，类型和 UI 都不用动。**
- `AccountTestModal.vue`：协议结果面板下方加「模型指纹测试」区块。一行样式对齐协议行
  （图标 + 名称 + 状态 + 耗时 + 独立按钮），行下的框放结论（最像的模型 + 概率、Top-5 候选概率条、
  证据、免责、原始返回折叠）。**默认不跑，也不随「开始测试」触发或清除，必须单独点「开始检测」。**
  顺带删掉了上游弹窗底部那行「测试模型 / 提示词：…」信息行（长空 2026-10-15 反馈它位置不对），
  它用的 `testModeSummary` computed 一并删除（`test-modal-remove-test-info`）。
- `AccountBatchTestModal.vue`：每个可测账号行加一条指纹结果行（**默认就铺好，状态「未检测」**，
  由底部「开始指纹检测」按钮触发），与后端的协议测试完全独立（不写回账号协议）。

#### 两个实测反直觉点（改这块代码时必须知道）

1. `evidence.insufficient` 在**成功**检测时也是 `true`——它表达的是「证据不足以证明身份」，
   不是「检测失败」，**UI 绝不能渲染成错误态**。
2. `analysis.results` 是参考库**全部**模型的完整排行（本次 59 条），不是 top-N，UI 要自己截断。

#### 后续（后端，尚未做）

- 运行时用 **Bun**（单文件、musl 版能直接塞进 alpine；Node 还要额外拖 libstdc++）
- 构建期烘焙 CLI 进镜像（改 `Dockerfile.goreleaser`；要往构建上下文放文件还得同步改
  `.github/release-tools/release_matrix.py`），运行期从 npm registry 拉样本库到
  `/app/data/lm-detector/bank/`，用 `--bank` 指过去——这样**不重新部署也能更新样本库**
- 后端 exec `fpd --json`，解析 `fpd-detection-v1` 报告
- 完整调研（CLI 参数、样本库在线源实测、sub2api 挂载点、镜像约束、硬限制）见
  `临时文件/lm-detector调研.md`

| 文件 | 锚点 |
|---|---|
| `frontend/src/components/admin/account/AccountTestModal.vue` | `test-modal-fingerprint-section` / `test-modal-fingerprint-state` / `test-modal-fingerprint-four-state` / `test-modal-fingerprint-reset-on-open` |
| `frontend/src/components/admin/account/AccountBatchTestModal.vue` | `batch-test-fingerprint-line` / `batch-test-fingerprint-field` / `batch-test-fingerprint-run` |
| `frontend/src/i18n/locales/zh/admin/accounts.ts` | `i18n-fingerprint-zh` |
| `frontend/src/i18n/locales/en/admin/accounts.ts` | `i18n-fingerprint-en` |
| `.../account/__tests__/AccountTestModal.spec.ts` | `test-test-modal-fingerprint` |
| `.../account/__tests__/AccountBatchTestModal.spec.ts` | `test-batch-test-fingerprint` |

### 2.26 平台显示名统一走平台清单（改上游文件，新增 24 个锚点、删 1 个）

**问题**（长空 2026-10-15 反馈）：二开新增的平台 `ag_hub` 在若干位置显示成原始 id `ag_hub`，
而不是「聚合中转」。

**长空定的规矩**：**所有 web 上显示的地方都要显示平台名，原始 id 只允许留在代码里。**

根因两类，都已修完：

**（1）本地硬编码的平台名 map 覆盖了 catalog 驱动的 helper** —— 这类最隐蔽，新增平台必然漏。
已删掉本地 map，改从 `@/utils/platformColors` 引 `platformLabel`
（内部走 `getPlatformSpec(p)?.display_name`，由后端平台清单驱动）：

| 文件 | 锚点 |
|---|---|
| `frontend/src/components/user/PlatformUsageBreakdown.vue` | `platform-usage-breakdown-display-name` |
| `frontend/src/components/user/dashboard/UserDashboardStats.vue` | `user-dashboard-stats-display-name` |
| `frontend/src/features/channel-monitor-v2/MonitorSettingsPanel.vue` | `monitor-settings-display-name` |

**（2）直接渲染原始 platform id** —— 批量测试弹窗分组头（`batch-test-platform-display-name`）
加 20 个文件（锚点 `platform-display-name-<文件路径去 .vue>`，逐个见 `锚点清单.md`），
统一改成 `platformLabel(x)`；账号/分组语境用
`t('admin.accounts.platforms.' + p, platformLabel(p))`。

**防回归守卫**：新增 `frontend/src/utils/__tests__/platformDisplayGuard.spec.ts`
（锚点 `test-platform-display-name-guard`）——扫全部 `.vue` 模板的 `{{ }}` 插值，
凡是把 platform 字段直接当文本渲染的就报错。以后谁再写裸插值会立刻红。
它**覆盖不到**：script 里拼模板字符串当 label 的、`:title` / `:label` 属性绑定、
以及 `getDimensionString(row, 'platform')` 这种按字符串取字段的——这些只能靠人眼。

**故意没改的两处**：`OpsSystemLogTable.vue:156` 和 `OpsAlertEventsCard.vue:196` 里的
`platform=<id>` 是**和后端日志做对应的关联串**，不是给人看的显示名。
改成中文会让前端拼出的 `platform=聚合中转` 对不上后端的 `platform=ag_hub`，属于功能回归。
长空如果仍要改，说一声。

**一个共性风险**：`platformLabel` 依赖平台清单，若清单在首屏渲染前还没加载完，
所有调用点都会短暂回落成原始 id。本次没有验证首屏时序。

### 2.27 编辑账号弹窗拆成两个 tab（改上游文件，新增 9 个锚点）

`frontend/src/components/account/EditAccountModal.vue` 的表单有三千多行，
配「临时不可调度」要滚很久。按 `ChannelsView.vue` 的「编辑渠道」弹窗做成两个 tab：

- `基础信息`（默认）/ `临时调度`，`activeTab` 状态在 `edit-account-tabs-state`；
- tab 条 `edit-account-tabs` 吸顶（`sticky top-0` + 负 margin 撑满弹窗宽度），滚到哪儿都能切；
- 原有表单内容整块包进 `edit-account-tab-panels`，临时不可调度整块包进 `edit-account-tab-temp-panel`；
- **用 `v-show` 不用 `v-if`**：既有的三条 temp-unsched 测试直接查 DOM 里的
  `[data-testid^="temp-unsched-scope-"]`，`v-if` 会把它们全打红；
- 每次打开或换账号时 `edit-account-tabs-reset` 把 tab 复位到「基础信息」；
- 样式 `edit-account-tabs-style` 新增了本文件第一个 `<style scoped>` 块，照抄 ChannelsView 的三个 class。

**没跟着搬的**：`账号调度阈值覆盖`（`account-scheduling-threshold-section`）留在「基础信息」。
长空只点名了「临时不可调度开关以及配置」，阈值覆盖是账号级语义，不扩范围。

**踩坑**：测试里不要用 `wrapper.isVisible()` 判断 tab 显隐。
它走 jsdom 的 `getComputedStyle`，该值会缓存，切完 tab 仍返回切换前的旧值，
表现为「点了没反应」的假失败。断言 `(el as HTMLElement).style.display !== 'none'`，
这才是 `v-show` 真正控制的东西（锚点 `test-edit-account-tabs`）。

**吸顶那条缝（2026-10-15 真机实测发现并修掉）**：`.modal-body` 自带
`padding-top`（12px / `sm:` 16px），吸顶的 tab 条盖不住那一条，滚过去的内容会从缝里
露出来。实测 tab 条停在 y=148，内容出现在 y=140。修法是给 `.edit-account-tabs`
加一个同底色的 `::before` 盖住那 12/16px（同一个 `edit-account-tabs-style` 锚点内）。

排查时注意：`elementFromPoint` **会跳过 `pointer-events: none` 的元素**，
拿它当探针会得出「没盖住」的错误结论；这条缝要靠截图或
`getComputedStyle(el, '::before')` 确认。

**真机验证记录（playwright-cli + headed Edge，2026-10-15）**：
tab 文案 `["基础信息","临时调度"]`、默认 `basic=shown / temp=hidden`、
切过去 `basic=hidden / temp=shown`、`aria-selected` 正确、`position: sticky` 生效、
基础信息面板里不含「临时不可调度」。测试账号 `#3354 临时鸡蛋1`（聚合中转 Key）
已经有 8 条以上临时不可调度规则 —— 正好是拆 tab 要解决的场景。

**i18n**：`admin.accounts.editTabs.basic` / `.tempScheduling`，
zh/en 各一个锚点（`i18n-edit-account-tabs-zh` / `-en`）。i18n 完整性测试卡着，两边都要加。

**同批改动**：`CLAUDE.md` 改名为 `AGENTS.md`，仓库内 7 处指向旧文件名的注释
和 `锚点清单.md` 一并更新；改名后该文件不再被上游 `.gitignore:122` 命中，不用再 `git add -f`。


### 2.28 模型指纹检测接后端（新增目录 + 改上游文件，新增 43 个锚点）

承接 §2.25：那一轮只做了前端 + mock，这一轮把后端真正接上 —— 后端 exec
[lm-detector](https://github.com/Ikaleio/lm-detector) 的 CLI 对账号发起真实检测，
并在「系统设置 → 功能开关」里加开关与样本库更新时间点。**长空 2026-10-15 定：CLI
版本钉死、构建期烘焙进镜像、样本库照常更新（不受检测开关影响）。**

#### 新增文件（都是 `FORK:` 头，不改上游）

| 文件 | 作用 |
|---|---|
| `backend/internal/service/fingerprint_service.go` | exec CLI、解析报告、base_url 规范化、协议映射、并发信号量 |
| `backend/internal/service/fingerprint_bank_service.go` | cron 定时更新样本库：下载 → 自检 → 整目录原子替换 |
| `backend/internal/service/fingerprint_settings.go` | 时间点规范化 / 校验 / cron 换算（纯函数） |
| `backend/internal/service/fingerprint_settings_test.go` | 上面那些纯函数的单测（`-tags=unit`） |

#### 实测结论（2026-10-15 本机验证，改这块前必读）

1. **不用 Bun，用 alpine 自带 Node 22**。`lmfpd` 的 engines 是 `node>=22`，
   alpine 3.21 的 `nodejs` 包正好是 22.23.2-r0。§2.25 里「Node 要拖 libstdc++
   所以选 Bun」的结论作废。
2. **`fpd.js` 顶层静态 import `ink` / `react` / `react/jsx-runtime` / `terminal-link`**。
   加 `--json` 跳 TUI 也绕不开，缺依赖直接崩。`npm install` 后 42 个包、19 MB、
   **零原生模块**（纯 JS，可跨平台烘焙）。
3. **网络失败时 fpd 仍输出合法 JSON**，退出码 1，**stderr 为空**。错误在
   `rounds[].samples[].error`，顶层 `scored_rounds` 是 0。所以**不能只看退出码**，
   必须解析 JSON 看 `scored_rounds`。
4. **`fpd --input <报告> --bank <库>` 可以离线自检样本库**，约 1 秒、不发网络请求。
   库与检测器不配套时（如截断 `shared_detector.bin`）退出码 1 且 stderr 给出
   `fpd retrain` 提示。样本库更新用它当准入门槛 —— 不合格的库不替换线上的。
   自检报告是**临时合成**的（`buildFingerprintSelfCheckProbe`），不随镜像烘焙。
5. 输出是**多行格式化 JSON**（约 1196 行 / 60 KB），按整段解析，别按行切。
6. `fpd.js` 用 `import.meta.url` 定位 `../data`，依赖靠 Node 向上找 `node_modules`，
   所以**保持包的原样布局**即可，不用单独拷 bin/data。

#### 挂载点

- 接口：`POST /api/v1/admin/accounts/:id/fingerprint`（非流式，一次 30-180 秒）。
  批量由前端按行并发调用，服务端信号量限 3 个并发进程。
- 路由：`backend/internal/server/routes/admin.go`；handler：
  `admin.AccountHandler.Fingerprint`（依赖经 `SetFingerprintService` 注入，
  和 `SetUpstreamBillingProbeService` 同一套写法）。
- 设置项走的是 `channel_monitor_enabled` 那套完整链路（DB key-value，无迁移）：
  `domain_constants` → `setting_parse` 默认值 + 解析 → `settings_view` 两个结构体
  → `setting_public` 读取 + SSR 注入 → `setting_update` 写库 → `dto/settings.go`
  → 三个 handler → 审计 diff。**`public_settings_injection_schema_test.go` 和
  `api_contract_test.go` 会卡住漏改**，两处 JSONEq 期望也要补字段。
- 样本库更新时间点在管理员保存设置后立即生效：`set-fingerprint-bank-service` +
  `update-reload-fingerprint-bank` 调 `FingerprintBankService.Reload(ctx)`
  （照 `OpsCleanupService` 的可 Reload 模式，不用 `sync.Once`）。
- 开关关闭时只影响「能不能发起检测」：前端 `featureFlags.ts` 的
  `fingerprint` flag（**opt-out**，默认开）控制指纹区块与批量按钮的渲染，
  后端 `GetFingerprintRuntime` 拒绝执行。**样本库照常更新。**

#### 镜像

`Dockerfile.goreleaser` 新增一个 `node:22-alpine` 构建阶段装 `lmfpd`（版本由
`ARG FINGERPRINT_CLI_VERSION` 钉死），整棵 `node_modules` 拷到
`/app/lm-detector/node_modules`；运行镜像 `apk add nodejs`（**必须构建期装**：
compose 带 `no-new-privileges`，运行期不能 apk add）。
**没有改 `release_matrix.py`** —— 构建上下文只多了一个 Dockerfile 里的 `COPY --from`。

运行期下载的样本库落在 `/app/data/lm-detector/bank/`（`--bank` 指过去）；
目录不存在或两个文件不齐时自动退回镜像里的内置库，所以更新失败不会中断检测。

#### 没做 / 已知边界

- 只支持 API Key 账号：OAuth、Bedrock/Vertex 云签名类账号没有 CLI 认得的
  base_url + key 组合，后端直接返回可读的拒绝理由。
- 一次检测真实向上游发 3 个请求（`--count 3`，少了没有置信度），**是要花钱的**。
- 残留风险：运行期样本库版本可能领先镜像里烘焙的 CLI 很多。自检能挡住
  「库读不出来」，挡不住「库的语义变了」。真出问题的回退动作是删掉
  `/app/data/lm-detector/bank/`，检测会退回内置库。


### 2.29 错误请求页「选号失败 / 未选号」+ 选号失败文案补原因（改上游文件，新增 7 个锚点）

**长空 2026-10-16 定：先提交，随 0.2.17 一起发版。** 起因与量化证据见
`工具/管理_sub2api/子任务/二开sub2api/临时文件/进度-错误请求账号列为空.md`。

问题：使用记录→错误请求（`/admin/ops/errors`）近 30 天 1801 行里 1159 行（64%）账号列为空，
前端一律显示 `-`。核查结论是**没丢数据**——这些行都是路由阶段就没选出账号；
但兜底文案 `Service temporarily unavailable` 与真正的上游错误 `Upstream service temporarily
unavailable` 只差一个词，被读成「某账号返回的上游错误」。

| # | 文件 | 改动 | 锚点 |
|---|---|---|---|
| 1 | `backend/internal/handler/no_account_error.go` | 新增 `selectionFailureMessage()`：非 404 时把选号器错误文案（含模型名与 `pool=/filtered:` 原因计数）接在分类文案后；分类文案为空时只回原因；原因已在文案里不重复追加 | `fork-selection-failure-reason` |
| 2 | `backend/internal/handler/openai_gateway_handler.go` | 2 处选号失败分支改用该函数（`Responses` 主路径、`Messages` 的 `/v1/messages` 路径） | `fork-selection-failure-reason` |
| 3 | `frontend/src/views/admin/ops/components/OpsErrorLogTable.vue` | 账号列无账号时不再显示 `-`：`phase=routing` → 琥珀色「选号失败」，其余 → 灰色「未选号」 | `fork-missing-account-badge` |
| 4 | `frontend/src/i18n/locales/{zh,en}/admin/ops.ts` | `errorLog.accountSelectionFailed` / `errorLog.accountNotSelected` | — |

404「分组里没有账号配该模型」不接后缀：分类文案本身已把话说全，选号器统计没有增量信息。

测试：`backend/internal/handler/fork_selection_failure_message_test.go`（6 用例，新建文件，带 `FORK:` 头）、
`frontend/src/views/admin/ops/components/__tests__/OpsErrorLogTable.spec.ts`（+3 用例与 i18n 键断言）。

**已知边界**：选号器只回裸错误 `no available accounts` 的行（近 30 天约 23 行），文案会是
`Service temporarily unavailable (selection failure: no available accounts)`，后缀没有信息量；
文案也只到「哪个模型、几个候选、为什么被排除」，不点名账号。要更准得让选号器把所有分支都带上统计。

## 3. 发版方式

版本号与上游保持一致，**不追加第四段**。

### 3.1 tag 策略：同名一律覆盖重建

**长空 2026-09-29 定：fork 是打补丁用的，tag 可以覆盖。**

- 从上游拉来的 `v0.2.9` 之类同名 tag，我们改动后**直接覆盖重建**，不需要新版本号；
- 每次发版都必须把 tag 指到**最新的发版提交**上，不允许留在旧提交；
- 实现方式：`release` 作业里的 `Reset release tag and release` 步骤，在构建镜像之后、发布 Release 之前执行

```bash
git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
MESSAGE="${TAG_MESSAGE:-Fork release ${RELEASE_VERSION}}"
git tag -f -a "$RELEASE_TAG" -m "$MESSAGE" "$RELEASE_SHA"
git push --force origin "refs/tags/$RELEASE_TAG"
gh release delete "$RELEASE_TAG" --yes --repo "$GITHUB_REPOSITORY" || true
```

覆盖前先把原 tag 的说明读进 `TAG_MESSAGE`，覆盖时用带注释的 tag（`-a`）重建，避免说明文字丢失。原 tag 没有说明时回退为 `Fork release <version>`。`git tag -a` 需要提交者身份，runner 上默认没有，所以必须先 `git config`（漏了会以 exit 128 失败，报 `empty ident name`）。

用 `GITHUB_TOKEN` 推送，GitHub 不会因此再触发一次 Release 工作流。删掉旧 Release 是为了让 goreleaser 能重新发布（同名 Release 已存在时会报错）。dry-run 不执行这一步。

手动打 tag 时也要遵守同一条：**先删后建，指向最新提交**。

```bash
git tag -d v0.2.9                                  # 删本地
git push origin :refs/tags/v0.2.9                  # 删远端
git tag -a v0.2.9 -m "同步上游 0.2.9 + 本地补丁"    # 在最新提交上重建
git push origin v0.2.9
```

### 3.2 手动触发发版（无 tag，本 fork 新增）

打完补丁、合并上游之后不想打 tag 时，用 `workflow_dispatch` + `use_version_file`：

```bash
gh workflow run release.yml -R hjkl950217/sub2api \
  --ref main \
  -f tag=main \
  -f use_version_file=true \
  -f simple_release=true
```

版本号取所选 ref 上的 `backend/cmd/server/VERSION`。tag 名优先用 HEAD 上已有的 `v*` tag，没有则回退为 `v<version>`；随后由 3.1 的步骤把它覆盖指到本次发版的提交。

> 注意：tag 名回退时它原本不存在，goreleaser 的 tag 消息为空，Release 说明会缺少正文。

### 3.3 仓库变量

| 变量 | 作用 | 当前 |
|---|---|---|
| `SIMPLE_RELEASE` | `true` 时只出 amd64 GHCR 镜像，跳过 arm64 和多架构 manifest | 未设置 |

仓库里**没有配置任何 secret**（`DOCKERHUB_*`、`TELEGRAM_*` 全为空），DockerHub 渠道已从工作流中移除，不依赖 secret 缺失来兜底。

## 4. 产物与部署

| 项 | 值 |
|---|---|
| 镜像仓库 | `ghcr.io/hjkl950217/sub2api`（GHCR 包默认私有，拉取需 `docker login ghcr.io`） |
| 镜像标签 | `:<version>`、`:latest`、`:0.2`、`:0`、`:<version>-amd64` / `-arm64` |
| 镜像内嵌前端补丁 | 有。两处补丁在 `build-frontend` 里打进 dist 再嵌入二进制，部署后**不需要**再往 `data/public/` 放覆盖文件 |
| 当前线上部署 | 已切到自建镜像 `ghcr.io/hjkl950217/sub2api:latest`（2026-09-29 切换），NAS compose 里指向它 |
| NAS | TrueNAS SCALE，Intel 12 代 x86_64，compose 在 `/mnt/nasData/dockerData/docker-compose-sub2api.yml` |

## 5. 已知风险

- `backend/internal/service/update_service.go` 里 `githubRepo = "Wei-Shaw/sub2api"` 是硬编码的，指向唯一上游，因此内置「检查更新」在 fork 上会报出上游新版本，点击更新会下载**上游二进制**覆盖当前实例。当前靠"版本号与上游一致"规避，一旦本地版本落后于上游，这个入口就会真的覆盖。
- `release-images.sh` 用 `${RELEASE_VERSION%%.*}` / `${RELEASE_VERSION#*.}` 截取 major/minor。版本号保持三段时正常；若将来出现第四段，会产出 `:0.2` 之外的脏标签。
- 运行时覆盖（`data/public/`）**优先级高于**内嵌补丁。NAS 上遗留的旧覆盖文件会盖掉镜像里的新补丁，切换镜像后要按子任务 `旧补丁/README.md` 的说明清理，否则会误判「补丁没生效」。
- 「分组模型账号」是**真路由页**（`/admin/group-model-accounts`），不依赖任何 DOM 注入。它随镜像发布，站点升级后不需要重建——升级后要重建的只剩 local-first 与监控排序两处。
- 2.5 的 `currentBaseHTML()` 每个首页请求都会 `os.Stat` 一次 `data/public/index.html`（约 1 次系统调用，远低于渲染与注入开销）；覆盖目录为空时走 `Stat` 失败的快路径，不会读文件。
- `patch_dist.py` 依赖 Vite 压缩后的函数形态。上游改了这个函数的写法，或者 Vite 升级换了压缩策略，模式就会失配——构建会在 `build-frontend` 阶段失败（不会静默出未打补丁的镜像）。

## 6. 修改本仓库的纪律

- 改动提交到 `main`；`main` 允许与上游分叉。
- **不要**执行 `git push --force` 到 `main`，也不要把 `main` reset 回上游——那会丢掉本节的改动。
- 上游的 `!拉取最新代码.ps1` 对含本地提交的分支执行 `git pull --ff-only` 会失败，这是预期行为，改用 `git fetch upstream && git merge upstream/main`。
- 每次合并上游后，按第 2 节的表格逐条复查改动是否被覆盖或变得多余。
- 改动工作流的 YAML 后，用下面这条命令确认能解析：

```bash
python -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml', encoding='utf-8'))"
```

## 7. 本机环境

开发机**没有 Docker、没有 make、没有 WSL**，无法构建镜像——镜像只能走 GitHub Actions。

Go 工具链装在 `工具/管理_sub2api/子任务/二开sub2api/临时文件/go/go/bin/go.exe`（1.27.0），能编译。
用法与踩坑见子任务 `旧补丁/记录_20261003_国产供应商协议复选与兜底转发协议.md` 第 5 节。两条现状：

- 上游 `backend/go.mod` 已要求 **go 1.27.2**，本机工具链是 1.27.0，必须把 `GOTOOLCHAIN` 设成 `auto`
  让它自己把 1.27.2 拉到 `临时文件/gopath/pkg/mod/golang.org/toolchain@...`；设 `local` 直接报
  `go.mod requires go >= 1.27.2`。
- **后端测试在 CI 跑，不在本机跑**（长空 2026-10-16 定，理由是本机内存不够）。
  本仓库 push/PR 触发的工作流从来不运行（CI / CLA Assistant / Security Scan 的运行数都是 0，
  只有手动触发的 Release 有记录，根因未查到），所以用：
  `gh workflow run backend-ci.yml -R hjkl950217/sub2api --ref main`，
  里面会跑 `make test-unit`（`go test -tags=unit ./...`）+ `make test-integration` + golangci-lint。

改 Go 代码前必须知道的格式坑（2026-10-16 踩过）：

- **gofmt 要用 go1.27.2 那一份**：`临时文件/gopath/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.2.windows-amd64/bin/gofmt.exe`。
  本机 1.27.0 的 gofmt 对齐与注释规则跟它不同，用它格式化出来的文件 CI 依然判不合格。
- **本机工作区是 CRLF，CI 看到的是 LF**，直接对工作区跑 `gofmt -l` 会误报一批文件。
  要判断 CI 会不会报，先把 HEAD 导成 LF 版再体检：
  `git archive HEAD backend | tar -x -C <临时目录>`，然后对导出的树跑 gofmt。
- gofmt 在仓库目录里**写不了临时文件**（`open <文件>.<随机数>: Access is denied`，本机沙箱拦新建）。
  绕法：把文件复制到临时目录 → 在那边 `gofmt -w` → 再拷回仓库。

本地能跑的是 `.github/release-tools/` 的 Python 单测：

```powershell
# Windows 下必须开 UTF-8 模式，否则读 .goreleaser.yaml 会因 GBK 解码报错
$env:PYTHONUTF8='1'
python -m unittest discover -s .github/release-tools -p 'test_release_matrix.py'
```

该套件在本机有 4 个用例必然失败，与代码改动无关：3 个依赖真实 `bash` 执行 `release-images.sh`（本机 `bash` 只是未安装 WSL 的占位程序），1 个依赖 POSIX 文件权限（NTFS 上 `chmod 0o755` 不生效）。在 ubuntu runner 上正常。
