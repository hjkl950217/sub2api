// FORK: 样本库定时更新（二开新增文件，不改上游）。
//
// lm-detector 的参考样本库随 npm 包 lmfpd 一起发布。镜像里烘焙的是构建那一刻的
// 版本，会越来越旧；这个服务按管理员在「系统设置 → 功能开关」里配的时间点，
// 每天去 npm registry 看看有没有新版本，有就把样本库换掉 —— **不用重新部署
// sub2api**（这是长空定的要求）。
//
// 与「模型检测」开关的关系：**样本库照常更新，不受那个开关影响**（长空 2026-10-15
// 明确）。开关只管「能不能发起检测」，不管「要不要保持样本库新鲜」。
//
// 安装前有一道自检（本机实测过）：临时造一份合成报告，用 `fpd --input` 对**新库**
// 重放一遍，约 1 秒、不发网络请求。库与检测器不匹配时 fpd 会硬失败并提示
// `fpd retrain`，这一步就是拿它当准入门槛 —— 不合格的库直接丢掉，不替换线上的，
// 避免「更新一次样本库，检测从此全挂」。
//
// 落盘用**整目录替换**：新库先写到同级的临时目录，校验通过后改名顶上。
// 两个文件（unified_bank.json + shared_detector.bin）必须成对，官方文档明确
// 「--bank 指向的库必须与同目录同名的 shared_detector.bin 配套」，只换一个会直接报错。
// 万一改名中途失败，目录不存在时检测会自动退回 CLI 内置库，不会读到半套库。
package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/robfig/cron/v3"
)

const (
	fingerprintBankJobName = "fingerprint_bank_sync"

	// 样本库里两个必须成对的文件（官方文档明确要同目录同名）。
	fingerprintBankDataFile     = "unified_bank.json"
	fingerprintBankDetectorFile = "shared_detector.bin"

	// fingerprintBankVersionFile 记录已安装样本库来自哪个 npm 版本，
	// 用来判断「要不要下载」以及排障时看现在装的是哪一版。
	fingerprintBankVersionFile = "version.txt"

	// fingerprintBankProbeFile 是自检时临时写进样本库目录的合成报告，
	// 跑完就删，不会跟着目录被顶上。
	fingerprintBankProbeFile = "self-check-probe.json"

	// 元数据请求超时；tarball 有 16 MB 左右，给宽一点。
	fingerprintBankMetaTimeout     = 30 * time.Second
	fingerprintBankDownloadTimeout = 5 * time.Minute
	// 自检超时：本机实测约 1 秒，给足余量但别拖住整个更新窗口。
	fingerprintBankSelfCheckTimeout = 60 * time.Second

	// fingerprintBankSelfCheckProbeCount 是自检合成报告里的样本点数。
	// 取和真实探测同量级的数字（真实一轮约 305 个），走同一条打分路径。
	fingerprintBankSelfCheckProbeCount = 305

	// 下载件的下限。样本库两个文件加起来约 16 MB，明显小于这个数说明下到了
	// 错误页或半截内容，直接判失败，不进入自检。
	fingerprintBankMinBytes = 4 << 20
)

// fingerprintBankCronParser 与仓库其它定时任务一致：标准五段，无秒字段。
var fingerprintBankCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// fingerprintBankRegistries 按顺序尝试的 npm registry。
//
// 本机实测（2026-10-15）：npmmirror 与 npmjs 都通；jsDelivr 被重置、
// GitHub raw 的大文件被中断，都不用作源。
var fingerprintBankRegistries = []string{
	"https://registry.npmmirror.com",
	"https://registry.npmjs.org",
}

// FingerprintBankService 按配置的时间点每天更新一次 lm-detector 样本库。
type FingerprintBankService struct {
	settingService *SettingService
	cfg            *config.Config

	// CLI 与目录位置。与 FingerprintService 保持同一套默认值和环境变量覆盖，
	// 因为两者必须指向同一份 CLI 与同一个样本库目录。
	nodePath  string
	cliScript string
	bankDir   string

	client *http.Client

	// mu 守护 cron 实例切换 + 生效时间点切换。
	// 不用 sync.Once：Reload 需要「停旧 cron 起新 cron」，Once 触发过就再也不执行。
	mu      sync.Mutex
	cron    *cron.Cron
	started bool
	stopped bool

	// running 防止上一轮还没跑完就叠加下一轮（下载 + 自检最长可达 6 分钟）。
	running atomicBool
}

// atomicBool 是个极小的原子布尔，避免为这一个标记引入 sync/atomic 的用法噪音。
type atomicBool struct {
	mu sync.Mutex
	v  bool
}

func (b *atomicBool) trySet() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.v {
		return false
	}
	b.v = true
	return true
}

func (b *atomicBool) clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.v = false
}

// NewFingerprintBankService 解析路径并返回服务实例。
func NewFingerprintBankService(
	settingService *SettingService,
	cfg *config.Config,
) *FingerprintBankService {
	cliDir := strings.TrimSpace(os.Getenv("SUB2API_FINGERPRINT_CLI_DIR"))
	if cliDir == "" {
		cliDir = fingerprintCLIDirDefault
	}
	bankDir := strings.TrimSpace(os.Getenv("SUB2API_FINGERPRINT_BANK_DIR"))
	if bankDir == "" {
		bankDir = fingerprintBankDirDefault
	}

	nodePath := "node"
	if resolved, err := exec.LookPath("node"); err == nil {
		nodePath = resolved
	}

	return &FingerprintBankService{
		settingService: settingService,
		cfg:            cfg,
		nodePath:       nodePath,
		cliScript:      filepath.Join(cliDir, "bin", "fpd.js"),
		bankDir:        bankDir,
		client:         &http.Client{},
	}
}

// ProvideFingerprintBankService 构造并启动样本库更新服务。
// 与仓库其它定时任务的接线方式一致：Provider 内直接 Start()，Stop 挂在
// cmd/server/wire.go 的 provideCleanup 里。
func ProvideFingerprintBankService(
	settingService *SettingService,
	cfg *config.Config,
) *FingerprintBankService {
	svc := NewFingerprintBankService(settingService, cfg)
	svc.Start()
	return svc
}

// Start 读取配置的时间点并启动 cron。幂等。
func (s *FingerprintBankService) Start() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.stopped {
		return
	}
	s.started = true
	if err := s.applyScheduleLocked(context.Background()); err != nil {
		logger.LegacyPrintf("service.fingerprint_bank", "[FingerprintBank] 启动失败: %v", err)
	}
}

// Stop 关闭 cron。幂等。
func (s *FingerprintBankService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	s.stopCronLocked()
}

// Reload 在管理员改了时间点后按新配置重建 cron。
// 服务未启动或已停止时什么都不做。
func (s *FingerprintBankService) Reload(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.stopped {
		return nil
	}
	return s.applyScheduleLocked(ctx)
}

func (s *FingerprintBankService) stopCronLocked() {
	if s.cron == nil {
		return
	}
	ctx := s.cron.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		logger.LegacyPrintf("service.fingerprint_bank", "[FingerprintBank] cron 停止超时")
	}
	s.cron = nil
}

// applyScheduleLocked 按当前设置的时间点重建 cron。调用方持锁。
func (s *FingerprintBankService) applyScheduleLocked(ctx context.Context) error {
	s.stopCronLocked()

	updateTime := FingerprintBankUpdateTimeDefault
	if s.settingService != nil {
		settings, err := s.settingService.GetAllSettings(ctx)
		if err != nil {
			// 读不到就按默认时间点跑，不要把定时任务整个停掉。
			logger.LegacyPrintf("service.fingerprint_bank",
				"[FingerprintBank] 读取设置失败，按默认时间点 %s 运行: %v", updateTime, err)
		} else {
			updateTime = settings.FingerprintBankUpdateTime
		}
	}

	spec, err := fingerprintBankCronSpec(updateTime)
	if err != nil {
		return err
	}

	loc := time.Local
	if s.cfg != nil && strings.TrimSpace(s.cfg.Timezone) != "" {
		if parsed, err := time.LoadLocation(strings.TrimSpace(s.cfg.Timezone)); err == nil && parsed != nil {
			loc = parsed
		}
	}

	c := cron.New(cron.WithParser(fingerprintBankCronParser), cron.WithLocation(loc))
	if _, err := c.AddFunc(spec, func() { s.runScheduled() }); err != nil {
		return fmt.Errorf("样本库更新时间点 %q 无法转成定时计划: %w", updateTime, err)
	}
	c.Start()
	s.cron = c

	logger.LegacyPrintf("service.fingerprint_bank",
		"[FingerprintBank] 已排定（每天 %s，cron=%q tz=%s）", updateTime, spec, loc.String())
	return nil
}

// runScheduled 是 cron 的触发入口。每次触发自建 context，和仓库其它定时任务一致。
func (s *FingerprintBankService) runScheduled() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	updated, err := s.Sync(ctx)
	if err != nil {
		logger.LegacyPrintf("service.fingerprint_bank", "[FingerprintBank] 更新失败: %v", err)
		return
	}
	if updated {
		logger.LegacyPrintf("service.fingerprint_bank", "[FingerprintBank] 样本库已更新")
	}
}

// Sync 检查并安装新样本库，返回是否发生了替换。
//
// 步骤：查最新版本 → 和本地已装版本比 → 下载 tarball → 取成对的两个数据文件 →
// 用 CLI 自检 → 整目录替换。任一步失败都不动线上样本库。
func (s *FingerprintBankService) Sync(ctx context.Context) (bool, error) {
	if s == nil {
		return false, errors.New("样本库更新服务未启用")
	}
	// 上一轮还没跑完就跳过这一轮，避免两个下载互相踩。
	if !s.running.trySet() {
		logger.LegacyPrintf("service.fingerprint_bank", "[FingerprintBank] 上一轮尚未结束，跳过本次")
		return false, nil
	}
	defer s.running.clear()

	version, tarballURL, err := s.fetchLatestVersion(ctx)
	if err != nil {
		return false, err
	}
	if version == s.installedVersion() {
		return false, nil
	}

	tarball, err := s.downloadTarball(ctx, tarballURL)
	if err != nil {
		return false, err
	}

	bankData, detector, err := extractBankFiles(tarball)
	if err != nil {
		return false, err
	}
	if len(bankData) < fingerprintBankMinBytes || len(detector) < fingerprintBankMinBytes {
		return false, fmt.Errorf("样本库文件大小异常（%d / %d 字节），疑似下载到错误内容",
			len(bankData), len(detector))
	}

	if err := s.installBank(ctx, version, bankData, detector); err != nil {
		return false, err
	}
	return true, nil
}

// fetchLatestVersion 依次尝试各 registry，返回版本号与 tarball 地址。
func (s *FingerprintBankService) fetchLatestVersion(ctx context.Context) (string, string, error) {
	var lastErr error
	for _, registry := range fingerprintBankRegistries {
		reqCtx, cancel := context.WithTimeout(ctx, fingerprintBankMetaTimeout)
		version, tarball, err := s.fetchLatestFrom(reqCtx, registry)
		cancel()
		if err == nil {
			return version, tarball, nil
		}
		lastErr = err
		logger.LegacyPrintf("service.fingerprint_bank",
			"[FingerprintBank] registry %s 不可用: %v", registry, err)
	}
	return "", "", fmt.Errorf("所有 npm registry 都不可用: %w", lastErr)
}

func (s *FingerprintBankService) fetchLatestFrom(ctx context.Context, registry string) (string, string, error) {
	url := strings.TrimRight(registry, "/") + "/lmfpd/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	// 元数据只有几百字节，限一下防止意外的大响应。
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", err
	}
	var meta struct {
		Version string `json:"version"`
		Dist    struct {
			Tarball string `json:"tarball"`
		} `json:"dist"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return "", "", fmt.Errorf("解析 registry 响应失败: %w", err)
	}
	if strings.TrimSpace(meta.Version) == "" || strings.TrimSpace(meta.Dist.Tarball) == "" {
		return "", "", errors.New("registry 响应里没有版本或下载地址")
	}
	return meta.Version, meta.Dist.Tarball, nil
}

func (s *FingerprintBankService) downloadTarball(ctx context.Context, url string) ([]byte, error) {
	reqCtx, cancel := context.WithTimeout(ctx, fingerprintBankDownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载样本库失败：HTTP %d", resp.StatusCode)
	}
	// 包体约 16 MB，留一倍余量。
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// extractBankFiles 从 npm tarball 里取出成对的两个样本库文件。
func extractBankFiles(tarball []byte) (bankData []byte, detector []byte, err error) {
	gz, err := gzip.NewReader(bytes.NewReader(tarball))
	if err != nil {
		return nil, nil, fmt.Errorf("样本库包不是合法的 gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()

	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("读取样本库包失败: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		// 包内路径形如 package/data/unified_bank.json，只用文件名匹配，
		// 不依赖目录前缀的具体写法。
		name := filepath.Base(header.Name)
		var target *[]byte
		switch name {
		case fingerprintBankDataFile:
			target = &bankData
		case fingerprintBankDetectorFile:
			target = &detector
		default:
			continue
		}
		// 单个文件约 4-12 MB，给 32 MB 上限防止异常包撑爆内存。
		content, err := io.ReadAll(io.LimitReader(reader, 32<<20))
		if err != nil {
			return nil, nil, fmt.Errorf("读取 %s 失败: %w", name, err)
		}
		*target = content
	}
	if len(bankData) == 0 || len(detector) == 0 {
		return nil, nil, errors.New("样本库包里没有找到 unified_bank.json 或 shared_detector.bin")
	}
	return bankData, detector, nil
}

// installBank 把新样本库写到临时目录、自检、再整目录替换上去。
func (s *FingerprintBankService) installBank(ctx context.Context, version string, bankData, detector []byte) error {
	parent := filepath.Dir(s.bankDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("创建样本库父目录失败: %w", err)
	}

	staging, err := os.MkdirTemp(parent, "bank-staging-")
	if err != nil {
		return fmt.Errorf("创建样本库临时目录失败: %w", err)
	}
	// 无论成败都不留临时目录；替换成功后它已被改名挪走，这里的删除会静默失败。
	defer func() { _ = os.RemoveAll(staging) }()

	for name, content := range map[string][]byte{
		fingerprintBankDataFile:     bankData,
		fingerprintBankDetectorFile: detector,
		fingerprintBankVersionFile:  []byte(version + "\n"),
	} {
		if err := os.WriteFile(filepath.Join(staging, name), content, 0o644); err != nil {
			return fmt.Errorf("写入样本库文件 %s 失败: %w", name, err)
		}
	}

	if err := s.selfCheck(ctx, staging); err != nil {
		return fmt.Errorf("新样本库未通过自检，保持原样本库不变: %w", err)
	}

	return swapBankDir(s.bankDir, staging)
}

// selfCheck 用一份**合成**报告对新样本库做一次离线重放。
//
// 这是准入检查，不是功能测试：它只验证「这份库能被当前烘焙的 CLI 读出来」。
// 库与检测器不匹配时 fpd 会直接失败，我们把这种情况挡在替换之前。
// 本机实测（2026-10-15）三种形态：
//   - 库与检测器配套 → 退出码 0，stdout 是带 scored_rounds 的合法报告
//   - shared_detector.bin 被截断 → 退出码 1，stdout 为空，stderr 提示 fpd retrain
//   - 库文件缺失 → 同样退出码 1
//
// 判定同时看退出码和 stdout：fpd 在网络失败时也会「退出码非 0 但 stdout 有合法
// JSON」，所以单看任何一边都不够。
//
// 报告是临时生成的，不随镜像烘焙。数字用固定步长造，不追求随机性 ——
// 自检只关心库能不能被解析，不关心分数。
func (s *FingerprintBankService) selfCheck(ctx context.Context, stagingDir string) error {
	probe, err := buildFingerprintSelfCheckProbe()
	if err != nil {
		return err
	}
	probePath := filepath.Join(stagingDir, fingerprintBankProbeFile)
	if err := os.WriteFile(probePath, probe, 0o644); err != nil {
		return fmt.Errorf("写入自检用合成报告失败: %w", err)
	}
	// 别让它跟着目录被顶上：自检通过后整个 staging 会改名成正式样本库目录。
	defer func() { _ = os.Remove(probePath) }()

	checkCtx, cancel := context.WithTimeout(ctx, fingerprintBankSelfCheckTimeout)
	defer cancel()

	// 自检虽是离线重放，也用同一套 node 启动参数，避免两条路径行为分叉
	// （见 fingerprintNodeFlags 里那条自动选族的说明）。
	args := append([]string{}, fingerprintNodeFlags...)
	args = append(args,
		s.cliScript,
		"--input", probePath,
		"--json",
		"--no-update-check",
		"--bank", filepath.Join(stagingDir, fingerprintBankDataFile),
	)
	cmd := exec.CommandContext(checkCtx, s.nodePath, args...)
	cmd.Env = append(os.Environ(), "FPD_NO_UPDATE_CHECK=1", "NO_UPDATE_NOTIFIER=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		// 无论退出码如何，stderr 有内容就是 fpd 在报错（它平时的输出走 stdout）。
		return errors.New(msg)
	}
	if err != nil {
		return fmt.Errorf("自检命令失败: %w", err)
	}

	var report struct {
		ScoredRounds int `json:"scored_rounds"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &report); err != nil {
		return fmt.Errorf("自检输出无法解析: %w", err)
	}
	if report.ScoredRounds == 0 {
		return errors.New("自检没有产生可评分的结果")
	}
	return nil
}

// buildFingerprintSelfCheckProbe 造一份最小的 fpd 报告，供 `fpd --input` 离线重放。
//
// 只需要 schema + 一轮带样本的 rounds：fpd 会拿这些数字去比对样本库，
// 从而把「库读不读得出来」暴露成退出码与 stderr。
func buildFingerprintSelfCheckProbe() ([]byte, error) {
	numbers := make([]string, 0, fingerprintBankSelfCheckProbeCount)
	for i := 0; i < fingerprintBankSelfCheckProbeCount; i++ {
		numbers = append(numbers, strconv.Itoa(1+(i*137+42)%355))
	}
	text := strings.Join(numbers, " ")

	probe := map[string]any{
		"schema":           "fpd-detection-v1",
		"created_at":       time.Now().UTC().Format(time.RFC3339),
		"requested_rounds": 1,
		"completed_rounds": 1,
		"scored_rounds":    1,
		"rounds": []any{map[string]any{
			"index":      1,
			"challenges": []any{map[string]any{"id": "self-check", "expected_count": fingerprintBankSelfCheckProbeCount, "prompt": ""}},
			"outputs":    []any{map[string]any{"text": text, "expected_count": fingerprintBankSelfCheckProbeCount}},
			"samples": []any{map[string]any{
				"state":         "complete",
				"text":          text,
				"rawText":       text,
				"count":         fingerprintBankSelfCheckProbeCount,
				"expectedCount": fingerprintBankSelfCheckProbeCount,
			}},
		}},
	}
	encoded, err := json.Marshal(probe)
	if err != nil {
		return nil, fmt.Errorf("生成自检报告失败: %w", err)
	}
	return encoded, nil
}

// swapBankDir 用 staging 目录顶掉线上的样本库目录。
//
// 先把旧的改名挪开再改名顶上：两次 rename 之间目录短暂不存在，此时检测会退回
// CLI 内置库（见 FingerprintService.resolveBankFile），读不到半套库。
func swapBankDir(bankDir, staging string) error {
	backup := bankDir + ".old"
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("清理上一版样本库备份失败: %w", err)
	}
	if _, err := os.Stat(bankDir); err == nil {
		if err := os.Rename(bankDir, backup); err != nil {
			return fmt.Errorf("移开旧样本库失败: %w", err)
		}
	}
	if err := os.Rename(staging, bankDir); err != nil {
		// 顶替失败就把旧的挪回来，别把功能留在「没有样本库」的状态。
		if _, statErr := os.Stat(backup); statErr == nil {
			if restoreErr := os.Rename(backup, bankDir); restoreErr != nil {
				logger.LegacyPrintf("service.fingerprint_bank",
					"[FingerprintBank] 样本库回滚失败，将退回内置库: %v", restoreErr)
			}
		}
		return fmt.Errorf("替换样本库失败: %w", err)
	}
	_ = os.RemoveAll(backup)
	return nil
}

// installedVersion 读当前已装样本库的版本号；没有或读不到时返回空串，
// 让调用方按「需要更新」处理。
func (s *FingerprintBankService) installedVersion() string {
	if s == nil {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(s.bankDir, fingerprintBankVersionFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}
