// FORK: 模型指纹检测 —— 后端 exec lm-detector CLI（二开新增文件，不改上游）。
//
// 前端「模型指纹测试」区块的数据来源。做法是把账号的 base_url / api_key / 协议
// 交给 npm 包 lmfpd 的命令行（bin/fpd.js），解析它输出的 fpd-detection-v1 报告。
//
// 三条实测事实（改这个文件前必须知道，2026-10-15 本机实测）：
//
//  1. **网络失败时 fpd 仍然输出合法 JSON**，退出码 1，stderr 是空的。错误在
//     rounds[].samples[].error 里，顶层 scored_rounds 是 0。所以**不能靠退出码判断
//     成败**，必须解析 JSON 看 scored_rounds。只看退出码会给出「检测失败但看起来
//     成功」的假象。
//  2. fpd.js 顶层静态 import ink / react / terminal-link，加 --json 跳 TUI 也没用，
//     依赖缺失直接崩。镜像里必须带这几个包（见 Dockerfile.goreleaser）。
//  3. 输出是格式化过的多行 JSON（实测 1196 行 / 60 KB），必须整段解析，不能按行切。
//
// 判定边界（UI 上也有对应免责文案）：lm-detector 是**参考库内的闭集排序**，
// 库外模型同样会给一个「最像」的候选。它回答「更像谁」，不回答「是不是谁」。
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	// fingerprintCLIDirDefault 是镜像里烘焙的 lm-detector 包位置。
	//
	// 指向 npm 装出来的包目录本身（而不是另拷一份 bin/data）：fpd.js 用
	// import.meta.url 定位 ../data，依赖也靠 Node 的 node_modules 向上查找，
	// 保持包的原样布局就不用管这两条解析规则。
	// Dockerfile.goreleaser 负责把它放到这里。
	fingerprintCLIDirDefault = "/app/lm-detector/node_modules/lmfpd"

	// fingerprintBankDirDefault 是运行期下载的样本库目录。
	// 非空时用 --bank 指过去；目录缺失或文件不全则退回 CLI 内置库。
	fingerprintBankDirDefault = "/app/data/lm-detector/bank"

	// fingerprintDefaultTimeout 是单次检测的整体超时。
	// fpd 默认每个请求 120s、3 个样本并发，正常一次 30-180s；批量时靠信号量排队。
	fingerprintDefaultTimeout = 300 * time.Second

	// fingerprintMaxConcurrency 限制同时运行的 fpd 进程数。
	// 每个 node 进程约 60-100 MB，且每次检测要向上游真实发 3 个请求——
	// 放开会让批量场景把宿主内存和上游限流一起打满。
	fingerprintMaxConcurrency = 3

	// fingerprintProbeCount 是 fpd 每轮生成的样本数。少于 3 就没有置信度，
	// 保持默认值 3，不暴露给前端。
	fingerprintProbeCount = 3
)

// FingerprintReport 是 fpd 输出的裁剪版报告，字段与前端 fingerprintMock.ts 的
// FingerprintReport 一一对应。刻意不带上游完整的 rounds / challenges / outputs
// （单次 60 KB，UI 不用），只留 UI 真正渲染的部分。
type FingerprintReport struct {
	Schema          string               `json:"schema"`
	CreatedAt       string               `json:"created_at"`
	Request         map[string]any       `json:"request"`
	Bank            FingerprintBankInfo  `json:"bank"`
	RequestedRounds int                  `json:"requested_rounds"`
	CompletedRounds int                  `json:"completed_rounds"`
	ScoredRounds    int                  `json:"scored_rounds"`
	Analysis        *FingerprintAnalysis `json:"analysis"`
}

type FingerprintBankInfo struct {
	ReferenceSHA256 string `json:"reference_sha256"`
	Models          int    `json:"models"`
}

type FingerprintAnalysis struct {
	Prediction           string                  `json:"prediction"`
	PredictionName       string                  `json:"prediction_name"`
	FamilyPrediction     string                  `json:"family_prediction"`
	FamilyPredictionName string                  `json:"family_prediction_name"`
	Probability          float64                 `json:"probability"`
	ProbabilityStatus    string                  `json:"probability_status"`
	ProbabilityScope     string                  `json:"probability_scope"`
	RankingScore         float64                 `json:"ranking_score"`
	UsedOutputs          int                     `json:"used_outputs"`
	Method               string                  `json:"method"`
	ModelVersion         map[string]any          `json:"model_version"`
	Calibration          map[string]any          `json:"calibration"`
	Results              []FingerprintCandidate  `json:"results"`
	Diagnostics          []FingerprintDiagnostic `json:"diagnostics"`
	Evidence             *FingerprintEvidence    `json:"evidence"`
}

type FingerprintCandidate struct {
	Model       string  `json:"model"`
	DisplayName string  `json:"display_name"`
	Family      string  `json:"family"`
	FamilyName  string  `json:"family_name"`
	Score       float64 `json:"score"`
	Probability float64 `json:"probability"`
}

type FingerprintDiagnostic struct {
	Index                int  `json:"index"`
	ParsedNumbers        int  `json:"parsed_numbers"`
	MinimumNumbers       int  `json:"minimum_numbers"`
	Accepted             bool `json:"accepted"`
	RawOutOfRangeCount   int  `json:"raw_out_of_range_count"`
}

type FingerprintEvidence struct {
	Insufficient bool     `json:"insufficient"`
	Label        string   `json:"label"`
	Reason       string   `json:"reason"`
	Threshold    *float64 `json:"threshold"`
	Method       string   `json:"method"`
}

// fingerprintRawReport 只用于取出失败原因。fpd 在失败时仍然输出完整报告，
// 但失败信息藏在 rounds 里，裁剪版报告不含这一段。
type fingerprintRawReport struct {
	ScoredRounds int `json:"scored_rounds"`
	Rounds       []struct {
		Error   string `json:"error"`
		Samples []struct {
			Error string `json:"error"`
		} `json:"samples"`
	} `json:"rounds"`
}

// FingerprintService 负责调用 lm-detector CLI 并解析结果。
type FingerprintService struct {
	accountRepo    AccountRepository
	settingService *SettingService
	cfg            *config.Config

	// nodePath / cliScript / bankDir 在构造时解析一次。
	// CLI 自身随镜像发布，路径不随运行期变化；样本库目录内容会变，但路径不变。
	nodePath  string
	cliScript string
	bankDir   string

	// sem 限制并发的 fpd 进程数，见 fingerprintMaxConcurrency。
	sem chan struct{}
}

// NewFingerprintService 解析 CLI 路径并返回服务实例。
//
// 环境变量覆盖（部署时排障用，正常不用设）：
//   - SUB2API_FINGERPRINT_CLI_DIR：CLI 目录
//   - SUB2API_FINGERPRINT_BANK_DIR：样本库目录
func NewFingerprintService(
	accountRepo AccountRepository,
	settingService *SettingService,
	cfg *config.Config,
) *FingerprintService {
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

	return &FingerprintService{
		accountRepo:    accountRepo,
		settingService: settingService,
		cfg:            cfg,
		nodePath:        nodePath,
		cliScript:      filepath.Join(cliDir, "bin", "fpd.js"),
		bankDir:        bankDir,
		sem:            make(chan struct{}, fingerprintMaxConcurrency),
	}
}

// ProvideFingerprintService 是 wire 的构造函数（与 NewFingerprintService 分开，
// 保持「Provider 函数名 = Provide*」的仓库惯例，便于在 ProviderSet 里登记）。
func ProvideFingerprintService(
	accountRepo AccountRepository,
	settingService *SettingService,
	cfg *config.Config,
) *FingerprintService {
	return NewFingerprintService(accountRepo, settingService, cfg)
}

// FingerprintRuntime 是「检测功能现在能不能用」的快照，供 handler 快速判断。
type FingerprintRuntime struct {
	Enabled bool
	// Ready 为 false 时 Reason 说明缺什么（CLI 没烘焙进镜像 / node 不在 PATH）。
	Ready  bool
	Reason string
}

// GetFingerprintRuntime 读取开关并检查 CLI 是否就位。
func (s *FingerprintService) GetFingerprintRuntime(ctx context.Context) FingerprintRuntime {
	runtime := FingerprintRuntime{Enabled: true}
	if s == nil {
		return FingerprintRuntime{Enabled: false, Reason: "模型指纹检测服务未启用"}
	}

	if s.settingService != nil {
		settings, err := s.settingService.GetAllSettings(ctx)
		if err != nil {
			// 读设置失败时保守放行：开关是 opt-out（默认开），读不到就不该把功能关掉，
			// 真正的失败会由 Detect 里的具体检查报出来。
			runtime.Enabled = true
		} else {
			runtime.Enabled = settings.FingerprintEnabled
		}
	}

	if !runtime.Enabled {
		return runtime
	}

	if _, err := os.Stat(s.cliScript); err != nil {
		runtime.Reason = fmt.Sprintf("镜像里没有找到 lm-detector CLI（%s）", s.cliScript)
		return runtime
	}
	if _, err := exec.LookPath(s.nodePath); err != nil {
		runtime.Reason = "容器里没有 node，无法运行 lm-detector CLI"
		return runtime
	}
	runtime.Ready = true
	return runtime
}

// Detect 对单个账号跑一次指纹检测，返回裁剪版报告。
//
// 调用方负责并发控制；本方法内部还有一层信号量，防止批量场景把进程数打满。
func (s *FingerprintService) Detect(ctx context.Context, accountID int64, modelID string) (*FingerprintReport, error) {
	if s == nil {
		return nil, errors.New("模型指纹检测服务未启用")
	}

	runtime := s.GetFingerprintRuntime(ctx)
	if !runtime.Enabled {
		return nil, errors.New("模型指纹检测已被管理员关闭")
	}
	if !runtime.Ready {
		return nil, errors.New(runtime.Reason)
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("账号不存在：%w", err)
	}

	baseURL, apiKey, err := fingerprintCredentials(account)
	if err != nil {
		return nil, err
	}

	model := strings.TrimSpace(modelID)
	if model == "" {
		model = fingerprintDefaultModel(account)
	}
	if model == "" {
		return nil, errors.New("该账号没有可用于指纹检测的模型，请在测试弹窗里选一个模型")
	}

	args := []string{
		s.cliScript,
		"-b", baseURL,
		"-m", model,
		"-a", fingerprintAPIFormat(account),
		"--json",
		"--count", fmt.Sprintf("%d", fingerprintProbeCount),
		"--timeout", "120",
		"--no-update-check",
	}
	if bankFile, ok := s.resolveBankFile(); ok {
		args = append(args, "--bank", bankFile)
	}

	// API key 走环境变量而不是命令行参数：命令行参数在 ps 输出里对同宿主其它进程可见。
	cmd := exec.CommandContext(ctx, s.nodePath, args...)
	cmd.Env = append(os.Environ(),
		"API_KEY="+apiKey,
		"FPD_NO_UPDATE_CHECK=1",
		"NO_UPDATE_NOTIFIER=1",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// 退出码 1 既可能是「上游不通」，也可能是「有轮次没排上名」——两种情况下
		// stdout 里都有合法 JSON，所以先尝试解析，解析不出来才按进程错误处理。
		if report, parseErr := parseFingerprintReport(stdout.Bytes()); parseErr == nil {
			return nil, fingerprintFailureError(stdout.Bytes(), report, stderr.String())
		}
		if ctx.Err() != nil {
			return nil, errors.New("检测超时或已取消")
		}
		return nil, fingerprintProcessError(err, stderr.String())
	}

	report, err := parseFingerprintReport(stdout.Bytes())
	if err != nil {
		return nil, err
	}
	if report.ScoredRounds == 0 {
		return nil, fingerprintFailureError(stdout.Bytes(), report, stderr.String())
	}
	return report, nil
}

// DetectWithTimeout 给 Detect 套一层默认超时，供 handler 直接调用。
func (s *FingerprintService) DetectWithTimeout(ctx context.Context, accountID int64, modelID string) (*FingerprintReport, error) {
	runCtx, cancel := context.WithTimeout(ctx, fingerprintDefaultTimeout)
	defer cancel()

	// 信号量在超时之外单独获取：排队等待不该吃掉检测本身的预算。
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-runCtx.Done():
		return nil, errors.New("检测排队超时，请稍后重试")
	}
	return s.Detect(runCtx, accountID, modelID)
}

// resolveBankFile 返回运行期下载的样本库文件路径。
//
// 成对约束（官方文档明确）：--bank 指向的库必须与同目录同名的 shared_detector.bin
// 配套。两个文件缺一就退回内置库 —— 只换一个会让检测直接报错，比退回更糟。
func (s *FingerprintService) resolveBankFile() (string, bool) {
	if s == nil || strings.TrimSpace(s.bankDir) == "" {
		return "", false
	}
	bankFile := filepath.Join(s.bankDir, "unified_bank.json")
	detectorFile := filepath.Join(s.bankDir, "shared_detector.bin")
	for _, path := range []string{bankFile, detectorFile} {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() == 0 {
			return "", false
		}
	}
	return bankFile, true
}

// fingerprintCredentials 取出 CLI 需要的 base_url 与 api_key。
//
// 只支持 API Key 账号：OAuth（Claude Code / Gemini）、Bedrock/Vertex 云签名类账号
// 没有 CLI 认得的 base_url + key 组合，直接给出可读的拒绝理由。
func fingerprintCredentials(account *Account) (baseURL string, apiKey string, err error) {
	if account == nil {
		return "", "", errors.New("账号不存在")
	}
	if account.Type != AccountTypeAPIKey {
		return "", "", errors.New("只有 API Key 账号能做指纹检测，OAuth 账号不支持")
	}
	apiKey = strings.TrimSpace(account.GetCredential("api_key"))
	if apiKey == "" {
		return "", "", errors.New("账号没有 API Key，无法检测")
	}
	baseURL = normalizeFingerprintBaseURL(account.GetBaseURL())
	if baseURL == "" {
		return "", "", errors.New("账号没有配置 base_url，无法检测")
	}
	return baseURL, apiKey, nil
}

// fingerprintEndpointSuffix 匹配 base_url 尾部的完整端点路径。
// fpd 会自己拼 /v1 + 端点，base_url 里带端点会直接报错，所以要先剥掉。
var fingerprintEndpointSuffix = regexp.MustCompile(`/(chat/completions|messages|responses)$`)

// normalizeFingerprintBaseURL 剥掉 base_url 尾部已有的端点路径与多余斜杠。
// 保留 /v1：fpd 只在 path 为空时才补 /v1，带 /v1 的地址它照样认。
func normalizeFingerprintBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimRight(trimmed, "/")
	trimmed = fingerprintEndpointSuffix.ReplaceAllString(trimmed, "")
	return strings.TrimRight(trimmed, "/")
}

// fingerprintAPIFormat 把 sub2api 的协议标识翻译成 fpd 的 -a 取值。
func fingerprintAPIFormat(account *Account) string {
	if account == nil {
		return "chatcompletion"
	}
	switch account.GetAPIProtocol() {
	case APIProtocolAnthropic:
		return "message"
	case APIProtocolResponses:
		return "responses"
	case APIProtocolChatCompletions:
		return "chatcompletion"
	default:
		// adaptive（按模型分流）没有单一协议。Anthropic 系账号走 message，
		// 其余（聚合中转八成是 OpenAI 兼容）走 chatcompletion。
		if account.Platform == PlatformAnthropic {
			return "message"
		}
		return "chatcompletion"
	}
}

// fingerprintDefaultModel 在调用方没指定模型时挑一个。
//
// 取 model_mapping 里最小的那个上游模型名：它是账号「声称提供」的模型，
// 正是要验证的对象。没有映射就返回空，由调用方报错让用户手选。
func fingerprintDefaultModel(account *Account) string {
	if account == nil {
		return ""
	}
	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		return ""
	}
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if value := strings.TrimSpace(mapping[key]); value != "" {
			return value
		}
	}
	return ""
}

// parseFingerprintReport 把 fpd 的 stdout 解析成裁剪版报告。
func parseFingerprintReport(raw []byte) (*FingerprintReport, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("lm-detector 没有输出任何内容")
	}
	var report FingerprintReport
	if err := json.Unmarshal(trimmed, &report); err != nil {
		return nil, fmt.Errorf("lm-detector 的输出不是合法 JSON：%w", err)
	}
	if report.Schema != "fpd-detection-v1" {
		return nil, fmt.Errorf("lm-detector 返回了未知的报告格式：%q", report.Schema)
	}
	return &report, nil
}

// fingerprintFailureError 从失败报告里拼出人话，说明为什么没测出结果。
//
// fpd 失败时仍然输出完整 JSON，原因藏在 rounds[].error 或更细的
// rounds[].samples[].error 里（实测：上游不可达时是 "fetch failed"）。
// 裁剪版报告没有这一段，所以要拿原始字节再解一次。
func fingerprintFailureError(raw []byte, report *FingerprintReport, stderr string) error {
	reasons := make([]string, 0, 4)
	var parsed fingerprintRawReport
	if err := json.Unmarshal(bytes.TrimSpace(raw), &parsed); err == nil {
		for _, round := range parsed.Rounds {
			// 轮级 error 说明「这一轮为什么没算出结果」（如 "No valid samples..."），
			// 样本级 error 才是根因（如 "fetch failed"）。两句都要，光有轮级那句
			// 管理员看不出是网络问题还是对方返回了垃圾。
			if msg := strings.TrimSpace(round.Error); msg != "" {
				reasons = append(reasons, msg)
			}
			for _, sample := range round.Samples {
				if msg := strings.TrimSpace(sample.Error); msg != "" {
					reasons = append(reasons, msg)
				}
			}
		}
	}
	if msg := strings.TrimSpace(stderr); msg != "" {
		reasons = append(reasons, msg)
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "上游没有返回可用的样本")
	}

	// 去重，同时保持出现顺序，避免 3 个样本报同一句 "fetch failed" 刷屏。
	seen := make(map[string]struct{}, len(reasons))
	unique := reasons[:0]
	for _, reason := range reasons {
		if _, ok := seen[reason]; ok {
			continue
		}
		seen[reason] = struct{}{}
		unique = append(unique, reason)
	}

	header := fmt.Sprintf("检测没有产生有效结果（%d/%d 轮完成，0 轮可评分）",
		report.CompletedRounds, report.RequestedRounds)
	return fmt.Errorf("%s：%s", header, strings.Join(unique, "；"))
}

// fingerprintProcessError 处理 CLI 进程本身起不来或崩掉的情况。
func fingerprintProcessError(runErr error, stderr string) error {
	if msg := strings.TrimSpace(stderr); msg != "" {
		return fmt.Errorf("lm-detector 执行失败：%w（%s）", runErr, msg)
	}
	return fmt.Errorf("lm-detector 执行失败：%w", runErr)
}
