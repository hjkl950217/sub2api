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

改动都提交在 `main` 分支。本节只放**台账索引**，逐项说明已拆到独立文件，别往 `AGENTS.md` 里塞：

| 文件 | 管什么 |
|---|---|
| `二开改动清单.md` | 每一处二开改动改了什么、改了哪些文件（原 §2.1–2.34 的逐项说明） |
| `合并判定清单.md` | 每次合并 / 吸收上游时，「上游语义 vs 二开语义」冲突处为什么这么判、改了哪条测试与哪个锚点 |
| `锚点清单.md` | `FORK-ANCHOR:` 的全量位置与数量校验命令 |

合并上游后，先按 `二开改动清单.md` 逐条复查改动是否还在，再跑 `锚点清单.md` 开头的统计命令对数量。

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
- 合并上游后先跑上面那条命令对数量（当前 **872 个** = 源码 852 + `.github/` 20），数量变少就是有改动被上游覆盖或冲突时被丢掉了。
  其中 `.github/` 占 20 个，单独看：`git -c core.quotepath=false grep -o "FORK-ANCHOR:" -- .github | wc -l`。

全量清单见 **`锚点清单.md`**：按文件分组列出每个锚点的名字，合并上游后照它逐条核对。

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

### 6.1 改代码与合并后的测试顺序（强制，长空 2026-11-23 定）

合并上游 0.2.24 时本机耗时 5.4 小时，其中 107 分钟耗在 26 轮 Go 全量单测上：改一处 → 跑全量 → 发现一处失败 → 再改 → 再跑全量。固定成下面三步：

1. **改完一处代码，当场评审对应测试**，不等全量报错。
   - 后端：`<file>.go` 找同目录 `<file>_test.go`；看上游本次是否新增或改动了针对该函数的断言，判断是代码错了还是测试期望错了，当场改。
   - 前端：`<file>.vue` / `<file>.ts` 找同目录 `__tests__/<file>.spec.ts` 或 `<file>.test.ts`。
   - 冲突文件里上游新增的测试优先看：`git show upstream/main:<test文件>` 比 HEAD 多出来的用例，本次测试失败绝大多数出在这里。
2. **只跑受影响的包/文件，本地不跑全量。**
   ```powershell
   # 后端：只跑改动涉及的包，再按函数名收窄
   $env:GOMAXPROCS = '4'
   go test -tags=unit -p 2 ./internal/service -run 'TestGateway|TestAnthropic' -count=1
   # 前端：只跑改动涉及的 spec
   $env:NODE_ENV = 'test'
   npx vitest run src/views/admin/__tests__/AccountsView.spec.ts
   ```
3. **全量测试不在本机跑，交给 CI**（长空 2026-11-23 定）：定向全绿后推送，`gh workflow run backend-ci.yml -R hjkl950217/sub2api --ref main` 跑 unit + integration + golangci-lint + 前端（含 `vue-tsc --noEmit` 与 `vitest`）。定向测试与上游期望冲突时，判定「保留二开改测试」还是「吸收上游改代码」，**判定结果写进 `合并判定清单.md`**（最上面追加一节，含依据与落地），不要写进本文件。

## 7. 本机环境

开发机**没有 Docker、没有 make、没有 WSL**，无法构建镜像——镜像只能走 GitHub Actions。

Go 工具链装在 `工具/管理_sub2api/子任务/二开sub2api/临时文件/go/go/bin/go.exe`（1.27.0），能编译。
用法与踩坑见子任务 `旧补丁/记录_20261003_国产供应商协议复选与兜底转发协议.md` 第 5 节。两条现状：

- 上游 `backend/go.mod` 已要求 **go 1.27.3**（0.2.19 起），本机工具链是 1.27.0，必须把 `GOTOOLCHAIN` 设成 `auto`
  让它自己把 1.27.3 拉到 `临时文件/gomodcache/golang.org/toolchain@...`；设 `local` 直接报
  `go.mod requires go >= 1.27.3`。`proxy.golang.org` 在本机连不上，拉工具链前要先设
  `$env:GOPROXY='https://goproxy.cn,direct'`。
- **本地只允许定向单测，禁止跑全量测试**（长空 2026-11-23 定，本机内存不够跑全量）；全量测试和 golangci-lint 交给 CI。
  本仓库 push/PR 触发的工作流从来不运行（CI / CLA Assistant / Security Scan 的运行数都是 0，
  只有手动触发的 Release 有记录，根因未查到），所以用：
  `gh workflow run backend-ci.yml -R hjkl950217/sub2api --ref main`，
  里面会跑 `make test-unit`（`go test -tags=unit ./...`）+ `make test-integration` + golangci-lint。
  本地定向测试的写法见 6.1。

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

## 8. Go 编译与测试的资源限制（强制，长空 2026-11-06 定）

跑测试的规则见 6.1（本地只跑定向，全量交给 CI），本节只讲资源上限。

本机 12 个逻辑核，`go build` / `go test` 默认按核数并行，单条命令就能吃满 CPU 并挤占内存。
所有 Go 命令一律带上限，**本地不跑 `go test ./...`**：

```powershell
$env:GOMAXPROCS = '4'   # 单个测试进程内的并发上限
go build -p 2 ./...
go test -tags=unit -p 2 ./internal/service -run TestXxx -count=1
```

- `-p` 管「同时编译/测试的包数」，`GOMAXPROCS` 管「单包内的并行」，两个都要设。
- 定向测试统一用 `-p 2`；全量（`./...`）不在本机跑。
- 需要硬隔离时用进程亲和性（compile 等子进程会继承）：
```powershell
$p = Start-Process go -ArgumentList 'test','-tags','unit','-p','2','./internal/service' -PassThru -NoNewWindow
$p.ProcessorAffinity = 0x00FF   # 只用逻辑核 0-7
$p.PriorityClass = 'BelowNormal'
$p.WaitForExit()
```
