# CLAUDE.md — sub2api fork

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

本文件本身也是改动之一：上游 `.gitignore:122` 忽略了 `CLAUDE.md`，所以它需要强制加入版本控制。

```bash
git add -f CLAUDE.md
```

### 2.0 标记纪律（强制，长空 2026-09-30 定）

**所有改动过的上游源码文件——前端和后端都算——必须在每处改动的紧邻位置带一个 `FORK-ANCHOR` 标记。**

```
// FORK-ANCHOR: <唯一名字> (<一句话说明这处改了什么>)
```

- 后端用 `//`，前端 `.vue/.ts` 用 `//`（模板里用 `<!-- FORK-ANCHOR: ... -->`），YAML 用 `#`；
- 一个标记对应一处**改动点**，不是整个文件一处；同一文件改了 3 处就要有 3 个标记；
- 标记名全局唯一，用下面这条命令列出全部改动点（**排除本文件**，本文件只是在文档里列举锚点名，不是改动点）：

  ```bash
  grep -rn "FORK-ANCHOR:" . --include="*.go" --include="*.vue" --include="*.ts" --include="*.yml" --include="*.py" --include="*.mjs" --include="*.css" | grep -v "^./CLAUDE.md"
  ```

- **新增的整文件**（`fork/` 下的东西、新增的视图/测试文件）不算「改上游」，不强制带标记，但文件头要有一行 `FORK:` 说明；
- 合并上游后先跑上面那条命令对数量（当前 **42 个**），数量变少就是有改动被上游覆盖或冲突时被丢掉了。

当前锚点全量清单（**42 个**，合并上游后逐个确认还在、且只出现一次）：

| 锚点 | 文件 |
|---|---|
| `override-first` / `override-index-loader` / `override-index-use` / `override-first-legacy` | `backend/internal/web/embed_on.go` |
| `sidebar-removed-icons` / `sidebar-announcements-entry` / `sidebar-redeem-promo-entries` / `sidebar-group-model-accounts` | `frontend/src/components/layout/AppSidebar.vue` |
| `route-group-model-accounts` | `frontend/src/router/index.ts` |
| `ci-header` / `ci-version-file-input` / `ci-version-file-plan` / `ci-patch-frontend-dist` / `ci-embed-fork-tests` / `ci-ghcr-only-build-binaries` / `ci-ghcr-push-only` / `ci-ghcr-only-release-env` / `ci-ghcr-only-dry-run` / `ci-recreate-release-tag` / `ci-no-dockerhub-description` / `ci-notify-dockerhub-empty` / `ci-disable-sync-version-file` | `.github/workflows/release.yml` |
| `release-matrix-version-file` | `.github/release-tools/release_matrix.py` |
| `release-matrix-version-file-tests` | `.github/release-tools/test_release_matrix.py` |
| `fork-embed-override-tests` | `backend/internal/web/embed_fork_test.go` |
| `account-search-by-id` | `backend/internal/repository/account_repo.go` |
| `account-search-by-id-local` / `account-search-debounce` / `account-search-debounce-handler` / `account-sort-default-id-desc` / `account-sort-default-id-desc-fallback` | `frontend/src/views/admin/AccountsView.vue` |
| `search-placeholder-id-zh` / `search-placeholder-id-en` | `frontend/src/i18n/locales/{zh,en}/admin/accounts.ts` |
| `account-search-single-trigger` | `frontend/src/components/admin/account/AccountTableFilters.vue` |
| `version-fork-config` / `version-disable-update` / `version-fork-repo-link` | `frontend/src/components/common/VersionBadge.vue` |
| `version-fork-i18n-zh` / `version-fork-i18n-en` | `frontend/src/i18n/locales/{zh,en}/misc.ts` |
| `sidebar-compact-width` | `frontend/src/components/layout/AppSidebar.vue` |
| `sidebar-compact-width-css` | `frontend/src/style.css` |
| `layout-compact-offset` | `frontend/src/components/layout/AppLayout.vue` |

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

上表 11 条对应的锚点在 `.github/workflows/release.yml` 里共 13 处（第 6、7 条各涉及多处 env），锚点全量清单见第 2.0 节。

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
| 3 | `VersionBadge.vue` | 「查看更新日志」下方新增「查看 fork 仓库」链接 | `version-fork-repo-link` |
| 4 | `{zh,en}/misc.ts` | `version` 段新增 `viewForkRepo`、`updateDisabledByFork` | `version-fork-i18n-zh` / `version-fork-i18n-en` |

只禁用了 UI 按钮，**后端接口未动**——直接调 `POST /admin/system/update` 仍会执行上游覆盖。要彻底封死得改 `update_service.go`，目前按「不扩大改动面」保留。

### 2.11 侧栏收窄（改上游文件，三处）

展开态侧栏从 16rem（`w-64`，256px）收到 **13rem（`w-52`，208px）**，正好容纳 8 个中文字（最长菜单项「分组模型账号」6 字），余量还给右侧内容区。

| # | 文件 | 改动 | 锚点 |
|---|---|---|---|
| 1 | `AppSidebar.vue` | `:class` 里 `w-64` → `w-52` | `sidebar-compact-width` |
| 2 | `style.css` | `.sidebar` 的 `@apply w-64` → `w-52`（与 1 保持一致，否则两处宽度说法不一） | `sidebar-compact-width-css` |
| 3 | `AppLayout.vue` | 内容区 `lg:ml-64` → `lg:ml-52` | `layout-compact-offset` |

208px 的算法：内容区 `px-3`(24) + 链接内边距(31) + 图标(20) + 间距(12) = 87px 固定开销，剩 121px 给文字，8 个 14px 汉字需 112px。改 `w-48`（192px）只剩 105px，8 字会被截断。

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

开发机**没有 Go、没有 Docker、没有 make、没有 WSL**，无法本地编译后端或构建镜像。后端改动的验证只能走 GitHub Actions。

本地能跑的是 `.github/release-tools/` 的 Python 单测：

```powershell
# Windows 下必须开 UTF-8 模式，否则读 .goreleaser.yaml 会因 GBK 解码报错
$env:PYTHONUTF8='1'
python -m unittest discover -s .github/release-tools -p 'test_release_matrix.py'
```

该套件在本机有 4 个用例必然失败，与代码改动无关：3 个依赖真实 `bash` 执行 `release-images.sh`（本机 `bash` 只是未安装 WSL 的占位程序），1 个依赖 POSIX 文件权限（NTFS 上 `chmod 0o755` 不生效）。在 ubuntu runner 上正常。
