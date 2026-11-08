# VLESS 隐藏上报功能 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 subagent-driven-development（推荐）或 executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为测速工具增加一个只有持有 NodeWeb 仓库令牌才能启用的 VLESS 订阅上报功能：拉取并解析 `config.js`，按测速结果生成 VLESS 链接，上传到 GitHub，并调用订阅接口取得订阅 URL。

**架构：** 后端新增 5 个仅回环可访问的 `/api/vless/*` 接口，复用现有 GitHub Contents API 与配置持久化；配置解析用 goja 在隔离 VM 中执行 `config.js`；链接生成、合并去重、订阅请求都是独立纯函数。前端新增一个隐藏的启用入口和一个独立的 VLESS 面板，原有界面与上报逻辑不改。

**技术栈：** Go 1.26、`github.com/dop251/goja`、GitHub REST API、原生 HTML/CSS/JS（WebView）。

**规格：** `docs/superpowers/specs/2026-11-09-vless-upload-design.md`

## 全局约束

- 原有 Worker / GitHub 上报、测速、CSV 导出、定时任务的字段和行为**不得改动**。
- 所有 `/api/vless/*` 只接受回环来源（`127.0.0.0/8`、`::1`），其他来源返回 403。
- 令牌只存服务端 `yx-config.json`（0600）；接口只回传 `has_*` 布尔，不返回明文。
- 默认不启用 ECH；ECH 只能由用户显式打开。
- 节点只取 `SpeedQualified=true` 的结果；数量 `0` 表示全部；按速度降序。
- 文件名默认 `vlesssub`；合并默认开启；合并按 `IP:端口` 去重。
- `config.js` 拉取响应上限 2MB；goja 执行超时 2 秒。
- 仓库格式必须为 `owner/repo`；文件路径禁止 `..`、禁止以 `/` 开头。
- 订阅接口只对 `worker.repo` 生效；上报到其他仓库时可以上传，但必须把订阅失败返回给前端。
- 测试命令：`go test ./internal/app/ -run <TestName> -v`；全量：`go test ./... -skip TestWritableDir`（Windows 下 `TestWritableDir` 是既有环境问题）。

## 基线

- 当前 HEAD：`5593a36 feat(upload): 增强 GitHub 上传功能的令牌兼容性与错误提示`，已包含 `normalizeGitHubToken`、`githubAuthHint`、`X-GitHub-Api-Version`。
- 工作区现有未提交的 `.gitignore`（忽略 `test/`）和 `docs/`（设计文档），先提交为基线。
- 复用 `internal/app/upload.go` 里的 `normalizeGitHubToken` / `githubAuthHint` / `truncate`，不重复实现。

## 审查重点（Review Focus）

1. **非回环访问**：`/api/vless/*` 从 `192.168.x.x` 等非回环地址调用时必须 403，不能因为页面能打开就放行。对应测试：`TestLocalOnlyMiddleware`（任务 8）。
2. **config.js 拉取失败**：无缓存时必须明确报错；有缓存时必须继续用缓存并在响应里带 `warning`，不能静默使用过期数据。对应测试：`TestFetchNodeWebConfigFallback`（任务 3）、`TestVlessUploadUsesCacheOnFetchFailure`（任务 10）。
3. **合并文件的杂行与重复**：远端文件含非法行、空行、IPv6 节点、重复 IP:端口时，非法行要保留，IPv6 键要正确，重复项要折叠/替换。对应测试：`TestMergeVlessLines`、`TestVlessNodeKeyIPv6`（任务 6）。
4. **没有可上报节点**：结果为空或全部未达速度门槛时必须返回 400，且**不得**发起 GitHub PUT，不能上传空文件。对应测试：`TestVlessUploadRejectsEmptyResults`（任务 10）。
5. **订阅失败不吞结果**：上报仓库与 `worker.repo` 不一致或订阅接口报错时，上传结果必须保留，`subscription_error` 要单独返回。对应测试：`TestVlessUploadSubscriptionFailure`（任务 10）。

## 文件结构

**创建：**

- `internal/app/github_file.go` — GitHub Contents API 的通用读写助手（GET/PUT + sha）。
- `internal/app/github_file_test.go` — 上面的 httptest 测试。
- `internal/app/vless_config.go` — NodeWeb 配置结构、RAW/Contents 拉取、goja 解析。
- `internal/app/vless_config_test.go` — 拉取与解析测试。
- `internal/app/vless_link.go` — VLESS 链接生成、节点键、合并去重、文件渲染。
- `internal/app/vless_link_test.go` — 链接与合并测试。
- `internal/app/vless_subscription.go` — 订阅链接接口客户端。
- `internal/app/vless_subscription_test.go` — 订阅接口测试。
- `internal/app/vless_server.go` — 5 个 VLESS 接口、回环中间件、状态结构。
- `internal/app/vless_server_test.go` — 接口测试（含 upload 的合并/失败路径）。
- `internal/app/web/vless.js` — 隐藏入口与 VLESS 面板的全部前端逻辑。
- `docs/superpowers/plans/2026-11-09-vless-upload.md` — 本文件。

**修改：**

- `internal/app/config.go` — 新增 VLESS 配置字段与默认值。
- `internal/app/server.go` — 注册 5 条路由（套回环中间件）。
- `internal/app/web/index.html` — 隐藏开发区块、顶栏入口、VLESS 弹窗、引入 `vless.js`。
- `internal/app/web/style.css` — VLESS 结果区样式。
- `go.mod` / `go.sum` — 新增 goja。

---

### 任务 0：基线整理

**文件：**
- 修改：`.gitignore`
- 创建：`docs/superpowers/specs/2026-11-09-vless-upload-design.md`（已存在）
- 创建：`docs/superpowers/plans/2026-11-09-vless-upload.md`（本文件）

- [ ] **步骤 1：确认工作区只有基线与文档改动**

运行：`git status --short`
预期：只有 `.gitignore` 修改和 `docs/` 未跟踪。

- [ ] **步骤 2：提交基线**

```bash
git add .gitignore docs/superpowers
git commit -m "docs: 添加 VLESS 上报功能设计文档与实现计划"
```

---

### 任务 1：配置字段与默认值

**文件：**
- 修改：`internal/app/config.go:7-38`（`Config` 结构）、`internal/app/config.go:44-56`（`DefaultConfig`）
- 测试：`internal/app/config_vless_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
func TestDefaultConfigHasVlessDefaults(t *testing.T) {
	c := DefaultConfig()
	if c.VlessConfigRepo != "Daizhouhui/NodeWeb" || c.VlessConfigBranch != "main" || c.VlessConfigPath != "config.js" {
		t.Fatalf("config source defaults wrong: %+v", c)
	}
	if c.VlessFile != "vlesssub" || !c.VlessMerge || c.VlessECH {
		t.Fatalf("vless upload defaults wrong: file=%q merge=%v ech=%v", c.VlessFile, c.VlessMerge, c.VlessECH)
	}
}

func TestLoadConfigKeepsVlessDefaultsForOldFiles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "yx-config.json")
	if err := os.WriteFile(p, []byte(`{"count":20}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c := loadConfigFrom(p)
	if c.VlessConfigRepo != "Daizhouhui/NodeWeb" || !c.VlessMerge || c.VlessFile != "vlesssub" {
		t.Fatalf("old config should inherit vless defaults: %+v", c)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/app/ -run 'TestDefaultConfigHasVlessDefaults|TestLoadConfigKeepsVlessDefaultsForOldFiles' -v`
预期：FAIL，报错 `c.VlessConfigRepo undefined`。

- [ ] **步骤 3：在 `Config` 中追加字段并补默认值**

字段（JSON tag 必须与下面一致）：

```go
VlessEnabled            bool   `json:"vless_enabled"`
VlessConfigRepo         string `json:"vless_config_repo"`
VlessConfigBranch       string `json:"vless_config_branch"`
VlessConfigPath         string `json:"vless_config_path"`
VlessConfigToken        string `json:"vless_config_token"`
VlessConfigCache        string `json:"vless_config_cache"`
VlessConfigFetchedAt    int64  `json:"vless_config_fetched_at"`
VlessUseConfigForUpload bool   `json:"vless_use_config_for_upload"`
VlessUploadRepo         string `json:"vless_upload_repo"`
VlessUploadToken        string `json:"vless_upload_token"`
VlessFile               string `json:"vless_file"`
VlessCount              int    `json:"vless_count"`
VlessMerge              bool   `json:"vless_merge"`
VlessECH                bool   `json:"vless_ech"`
VlessUUID               string `json:"vless_uuid"`
VlessHost               string `json:"vless_host"`
VlessSNI                string `json:"vless_sni"`
VlessPath               string `json:"vless_path"`
VlessFP                 string `json:"vless_fp"`
VlessSubscriptionURL    string `json:"vless_subscription_url"`
```

`DefaultConfig()` 追加：`VlessConfigRepo: "Daizhouhui/NodeWeb"`、`VlessConfigBranch: "main"`、`VlessConfigPath: "config.js"`、`VlessFile: "vlesssub"`、`VlessMerge: true`。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/app/ -run 'TestDefaultConfigHasVlessDefaults|TestLoadConfigKeepsVlessDefaultsForOldFiles' -v`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/app/config.go internal/app/config_vless_test.go
git commit -m "feat(vless): 增加 VLESS 上报配置字段与默认值"
```

---

### 任务 2：GitHub 文件读写助手

**文件：**
- 创建：`internal/app/github_file.go`
- 测试：`internal/app/github_file_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
func TestGithubGetFile(t *testing.T) {
	var gotAuth, gotPath, gotRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotRef = r.Header.Get("Authorization"), r.URL.Path, r.URL.Query().Get("ref")
		_, _ = w.Write([]byte(`{"content":"aGVsbG8=","sha":"abc","encoding":"base64"}`))
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	data, sha, err := githubGetFile(context.Background(), "owner/repo", "main", "sub/a.txt", "tok")
	if err != nil || string(data) != "hello" || sha != "abc" {
		t.Fatalf("got data=%q sha=%q err=%v", data, sha, err)
	}
	if gotAuth != "Bearer tok" || gotPath != "/repos/owner/repo/contents/sub/a.txt" || gotRef != "main" {
		t.Fatalf("request wrong: auth=%q path=%q ref=%q", gotAuth, gotPath, gotRef)
	}
}

func TestGithubGetFileNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	data, sha, err := githubGetFile(context.Background(), "owner/repo", "main", "nope.txt", "tok")
	if err != nil || data != nil || sha != "" {
		t.Fatalf("404 should be (nil, \"\", nil), got data=%q sha=%q err=%v", data, sha, err)
	}
}

func TestGithubPutFileSendsShaAndContent(t *testing.T) {
	var method, path string
	var body map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"content":"b2xk","sha":"sha1","encoding":"base64"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	old := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = old }()

	if err := githubPutFile(context.Background(), "owner/repo", "main", "a.txt", "tok", "msg", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPut || path != "/repos/owner/repo/contents/a.txt" {
		t.Fatalf("wrong request: %s %s", method, path)
	}
	if body["sha"] != "sha1" || body["branch"] != "main" || body["message"] != "msg" || body["content"] != base64.StdEncoding.EncodeToString([]byte("new")) {
		t.Fatalf("payload wrong: %+v", body)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/app/ -run 'TestGithubGetFile|TestGithubPutFile' -v`
预期：FAIL，`undefined: githubAPIBase` / `undefined: githubGetFile`。

- [ ] **步骤 3：实现 `github_file.go`**

签名：

```go
var githubAPIBase = "https://api.github.com"

// githubGetFile 读取文件内容与 sha；文件不存在返回 (nil, "", nil)。
func githubGetFile(ctx context.Context, repo, branch, path, token string) ([]byte, string, error)

// githubPutFile 创建或更新文件；已存在时自动带 sha。
func githubPutFile(ctx context.Context, repo, branch, path, token, message string, data []byte) error
```

实现要点：
- URL：`githubAPIBase + "/repos/" + repo + "/contents/" + escapedPath`，`branch` 非空时追加 `?ref=`。
- `escapedPath` 按 `/` 分段 `url.PathEscape`。
- 头：`Authorization: Bearer <normalizeGitHubToken(token)>`、`Accept: application/vnd.github+json`、`X-GitHub-Api-Version: 2022-11-28`。
- GET：404 返回 `(nil,"",nil)`；401/403 用 `githubAuthHint`；200 解析 `{content, sha}` 并 base64 解码。
- PUT：先调用 `githubGetFile` 拿 sha；payload `{message, content, branch, sha?}`；非 2xx 用 `githubAuthHint` 或 `fmt.Errorf("GitHub 写入失败 HTTP %d: %s", ...)`。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/app/ -run 'TestGithubGetFile|TestGithubPutFile' -v`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/app/github_file.go internal/app/github_file_test.go
git commit -m "feat(vless): 添加 GitHub Contents 文件读写助手"
```

---

### 任务 3：config.js 拉取（RAW 优先 + Contents 兜底）

**文件：**
- 创建：`internal/app/vless_config.go`
- 测试：`internal/app/vless_config_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
const testConfigJS = `window.NODE_WEB_CONFIG = {
  defaultVlessConfig: { uuid: "uuid-1", host: "h.example", sni: "s.example", path: "/?ed=2048", fp: "chrome", ech: "ech.example+https://dns.example/q", port: 443, encryption: "none", security: "tls", network: "ws" },
  github: { repo: "Owner/Sub", branch: "main", worker: { baseUrl: "https://manager.example", repo: "Owner/Sub", subscriptionApiPath: "/api/subscription-link" } },
  vlessPresets: { uuid: ["uuid-1"], host: ["h.example"], sni: ["s.example"], path: ["/?ed=2048"], fp: ["chrome"] }
};`

func TestFetchNodeWebConfigRawFirst(t *testing.T) {
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.URL.Path != "/Owner/NodeWeb/main/config.js" {
			t.Fatalf("raw request wrong: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(testConfigJS))
	}))
	defer raw.Close()
	oldRaw := rawGithubBase
	rawGithubBase = raw.URL
	defer func() { rawGithubBase = oldRaw }()

	data, err := fetchNodeWebConfig(context.Background(), "Owner/NodeWeb", "main", "config.js", "tok")
	if err != nil || string(data) != testConfigJS {
		t.Fatalf("got %q, %v", data, err)
	}
}

func TestFetchNodeWebConfigFallsBackToContentsAPI(t *testing.T) {
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer raw.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":"` + base64.StdEncoding.EncodeToString([]byte(testConfigJS)) + `","sha":"s","encoding":"base64"}`))
	}))
	defer api.Close()
	oldRaw, oldAPI := rawGithubBase, githubAPIBase
	rawGithubBase, githubAPIBase = raw.URL, api.URL
	defer func() { rawGithubBase, githubAPIBase = oldRaw, oldAPI }()

	data, err := fetchNodeWebConfig(context.Background(), "Owner/NodeWeb", "main", "config.js", "tok")
	if err != nil || string(data) != testConfigJS {
		t.Fatalf("fallback failed: %q, %v", data, err)
	}
}

func TestFetchNodeWebConfigBothFail(t *testing.T) {
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer raw.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer api.Close()
	oldRaw, oldAPI := rawGithubBase, githubAPIBase
	rawGithubBase, githubAPIBase = raw.URL, api.URL
	defer func() { rawGithubBase, githubAPIBase = oldRaw, oldAPI }()

	if _, err := fetchNodeWebConfig(context.Background(), "Owner/NodeWeb", "main", "config.js", "tok"); err == nil {
		t.Fatal("both sources failing must return an error")
	}
}

func TestFetchNodeWebConfigRejectsOversize(t *testing.T) {
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), maxConfigBytes+1))
	}))
	defer raw.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer api.Close()
	oldRaw, oldAPI := rawGithubBase, githubAPIBase
	rawGithubBase, githubAPIBase = raw.URL, api.URL
	defer func() { rawGithubBase, githubAPIBase = oldRaw, oldAPI }()

	if _, err := fetchNodeWebConfig(context.Background(), "Owner/NodeWeb", "main", "config.js", "tok"); err == nil {
		t.Fatal("oversize config must fail")
	}
}
```

测试文件导入：`bytes`、`context`、`encoding/base64`、`net/http`、`net/http/httptest`、`strings`、`testing`。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/app/ -run 'TestFetchNodeWebConfig' -v`
预期：FAIL，`undefined: rawGithubBase` / `undefined: fetchNodeWebConfig`。

- [ ] **步骤 3：实现拉取**

签名与常量：

```go
var rawGithubBase = "https://raw.githubusercontent.com"
const maxConfigBytes = 2 << 20

func fetchNodeWebConfig(ctx context.Context, repo, branch, path, token string) ([]byte, error)
```

实现要点：
- RAW URL：`rawGithubBase + "/" + repo + "/" + branch + "/" + escapedPath`；`Authorization: Bearer <normalized>`、`Accept: text/plain`。
- 200 → `io.LimitReader(resp.Body, maxConfigBytes+1)` 读取；超过 `maxConfigBytes` 返回错误；返回原文。
- 失败（非 2xx 或网络错误）→ 调用 `githubGetFile(ctx, repo, branch, path, token)` 作为兜底；成功且非空则返回。
- 两者都失败 → 返回包含「拉取 config.js 失败」与权限提示的错误。
- 仓库/分支/路径做基本校验：`repo` 必须包含一个 `/`；`branch`、`path` 非空；`path` 不含 `..`。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/app/ -run 'TestFetchNodeWebConfig' -v`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/app/vless_config.go internal/app/vless_config_test.go
git commit -m "feat(vless): 拉取 NodeWeb config.js（RAW 优先，Contents 兜底）"
```

---

### 任务 4：config.js 解析（goja）

**文件：**
- 修改：`internal/app/vless_config.go`
- 修改：`go.mod` / `go.sum`
- 测试：`internal/app/vless_config_test.go`

- [ ] **步骤 1：加入依赖**

运行：`go get github.com/dop251/goja@latest && go mod tidy`
预期：`go.mod` 出现 goja。

- [ ] **步骤 2：编写失败的测试**

```go
func TestParseNodeWebConfig(t *testing.T) {
	cfg, err := parseNodeWebConfig([]byte(testConfigJS))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultVless.UUID != "uuid-1" || cfg.DefaultVless.Host != "h.example" || cfg.DefaultVless.Path != "/?ed=2048" {
		t.Fatalf("defaults wrong: %+v", cfg.DefaultVless)
	}
	if cfg.Github.Worker.Repo != "Owner/Sub" || cfg.Github.Worker.SubscriptionAPIPath != "/api/subscription-link" {
		t.Fatalf("worker wrong: %+v", cfg.Github.Worker)
	}
	if len(cfg.VlessPresets.Host) != 1 || cfg.VlessPresets.Host[0] != "h.example" {
		t.Fatalf("presets wrong: %+v", cfg.VlessPresets)
	}
}

func TestParseNodeWebConfigRejectsBadScript(t *testing.T) {
	if _, err := parseNodeWebConfig([]byte(`window.NODE_WEB_CONFIG = {`)); err == nil {
		t.Fatal("invalid JS must fail")
	}
	if _, err := parseNodeWebConfig([]byte(`window.OTHER = {}`)); err == nil {
		t.Fatal("missing NODE_WEB_CONFIG must fail")
	}
}
```

- [ ] **步骤 3：运行测试验证失败**

运行：`go test ./internal/app/ -run 'TestParseNodeWebConfig' -v`
预期：FAIL，`undefined: parseNodeWebConfig`。

- [ ] **步骤 4：实现结构体与解析**

```go
type vlessDefaults struct {
	UUID          string `json:"uuid"`
	Host          string `json:"host"`
	SNI           string `json:"sni"`
	Path          string `json:"path"`
	FP            string `json:"fp"`
	ECH           string `json:"ech"`
	Port          int    `json:"port"`
	Prefix        string `json:"prefix"`
	Encryption    string `json:"encryption"`
	Security      string `json:"security"`
	Network       string `json:"network"`
	Insecure      bool   `json:"insecure"`
	AllowInsecure bool   `json:"allowInsecure"`
	ForcePort443  bool   `json:"forcePort443"`
}

type vlessPresets struct {
	UUID []string `json:"uuid"`
	Host []string `json:"host"`
	SNI  []string `json:"sni"`
	Path []string `json:"path"`
	FP   []string `json:"fp"`
}

type nodeWebConfig struct {
	DefaultVless vlessDefaults `json:"defaultVlessConfig"`
	Github       struct {
		Repo   string `json:"repo"`
		Branch string `json:"branch"`
		Worker struct {
			BaseURL             string `json:"baseUrl"`
			Repo                string `json:"repo"`
			SubscriptionAPIPath string `json:"subscriptionApiPath"`
		} `json:"worker"`
	} `json:"github"`
	VlessPresets vlessPresets `json:"vlessPresets"`
}

func parseNodeWebConfig(raw []byte) (*nodeWebConfig, error)
```

实现要点：
- `vm := goja.New()`；`vm.Set("window", vm.NewObject())`。
- 2 秒超时：`timer := time.AfterFunc(2*time.Second, func(){ vm.Interrupt("timeout") })`，`defer timer.Stop()`。
- `vm.RunString(string(raw))`；`vm.RunString("JSON.stringify(window.NODE_WEB_CONFIG)")`。
- 如果取到 `undefined` 或结果为空，返回「config.js 里没有 window.NODE_WEB_CONFIG」。
- 用 `encoding/json` 反序列化；返回错误时带上原因。
- 校验 `UUID/Host/SNI` 非空；为空返回明确错误。

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./internal/app/ -run 'TestParseNodeWebConfig' -v`
预期：PASS。

- [ ] **步骤 6：Commit**

```bash
git add go.mod go.sum internal/app/vless_config.go internal/app/vless_config_test.go
git commit -m "feat(vless): 使用 goja 解析 NodeWeb config.js"
```

---

### 任务 5：VLESS 链接生成

**文件：**
- 创建：`internal/app/vless_link.go`
- 测试：`internal/app/vless_link_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
func TestBuildVlessLinkMatchesExample(t *testing.T) {
	o := vlessLinkOptions{
		UUID: "99999999-9999-8999-9999-99abcdefabcd", Host: "lin.kyma.dpdns.org",
		SNI: "lin.kyma.dpdns.org", Path: "/?ed=2048", FP: "chrome",
		Encryption: "none", Security: "tls", Network: "ws",
	}
	r := Result{IP: "104.18.28.101", Port: 443, Speed: 1.55, Colo: "LAX"}
	got, err := buildVlessLink(o, r)
	if err != nil {
		t.Fatal(err)
	}
	want := "vless://99999999-9999-8999-9999-99abcdefabcd@104.18.28.101:443?encryption=none&security=tls&sni=lin.kyma.dpdns.org&fp=chrome&type=ws&host=lin.kyma.dpdns.org&path=%2F%3Fed%3D2048#%E6%B4%9B%E6%9D%89%E7%9F%B6-1.55MB%2Fs-443-104.18.28.101"
	if got != want {
		t.Fatalf("link mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestBuildVlessLinkForcePort443(t *testing.T) {
	o := vlessLinkOptions{
		UUID: "u", Host: "h", SNI: "s", Path: "/", FP: "chrome",
		Encryption: "none", Security: "tls", Network: "ws", ForcePort443: true,
	}
	r := Result{IP: "1.1.1.1", Port: 2053, Speed: 5, Colo: "HKG"}
	got, _ := buildVlessLink(o, r)
	if !strings.Contains(got, "@1.1.1.1:443?") || !strings.HasSuffix(got, "-443-1.1.1.1") {
		t.Fatalf("forcePort443 not applied: %s", got)
	}
}

func TestBuildVlessLinkIPv6(t *testing.T) {
	o := vlessLinkOptions{
		UUID: "u", Host: "h", SNI: "s", Path: "/", FP: "chrome",
		Encryption: "none", Security: "tls", Network: "ws",
	}
	got, _ := buildVlessLink(o, Result{IP: "2001:db8::1", Port: 443, Speed: 1, Colo: ""})
	if !strings.Contains(got, "@[2001:db8::1]:443?") {
		t.Fatalf("ipv6 host not bracketed: %s", got)
	}
}

func TestBuildVlessLinkECH(t *testing.T) {
	o := vlessLinkOptions{
		UUID: "u", Host: "h", SNI: "s", Path: "/", FP: "chrome",
		Encryption: "none", Security: "tls", Network: "ws",
		ECH: "cloudflare-ech.com+https://223.5.5.5/dns-query", UseECH: true,
	}
	got, err := buildVlessLink(o, Result{IP: "1.1.1.1", Port: 443, Speed: 1, Colo: "HKG"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "&ech=cloudflare-ech.com%2Bhttps%3A%2F%2F223.5.5.5%2Fdns-query#") {
		t.Fatalf("ech missing or wrong: %s", got)
	}
	o.ECH = ""
	if _, err := buildVlessLink(o, Result{IP: "1.1.1.1", Port: 443}); err == nil {
		t.Fatal("ECH enabled without ech value must fail")
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/app/ -run 'TestBuildVlessLink' -v`
预期：FAIL，`undefined: vlessLinkOptions`。

- [ ] **步骤 3：实现链接生成**

```go
type vlessLinkOptions struct {
	UUID, Host, SNI, Path, FP, ECH string
	Encryption, Security, Network  string
	ForcePort443, AllowInsecure    bool
	UseECH                         bool
}

// vlessOptionsFrom 用 config 默认值填充，并用 overrides 里的非空值覆盖。
func vlessOptionsFrom(def vlessDefaults, overrides vlessLinkOptions) vlessLinkOptions

func buildVlessLink(o vlessLinkOptions, r Result) (string, error)
```

`vlessOptionsFrom` 的测试：

```go
func TestVlessOptionsFrom(t *testing.T) {
	def := vlessDefaults{
		UUID: "d-uuid", Host: "d-host", SNI: "d-sni", Path: "/d", FP: "chrome",
		Encryption: "none", Security: "tls", Network: "ws",
	}
	o := vlessOptionsFrom(def, vlessLinkOptions{UUID: "x-uuid", Host: ""})
	if o.UUID != "x-uuid" || o.Host != "d-host" || o.Path != "/d" || o.Encryption != "none" {
		t.Fatalf("override merge wrong: %+v", o)
	}
}
```

实现要点：
- 校验 `UUID/Host/SNI` 非空；`Encryption/Security/Network` 为空时分别用 `none/tls/ws`。
- 端口：`r.Port`，`o.ForcePort443` 时 443；`<=0` 时 443。
- 主机：含 `:` 的 IPv6 写成 `[ip]`。
- 参数顺序：`encryption`、`security`、`sni`、`fp`、`type`、`host`、`path`。
- `path` 用 `url.QueryEscape`；`UseECH` 时追加 `ech`（同样转义），空则报错；`AllowInsecure` 时追加 `allowInsecure=1`。
- 别名：`region := ColoName(r.Colo)`；`region == "未知" || region == ""` 时用 `"未知地区"`；`fmt.Sprintf("%s-%.2fMB/s-%d-%s", region, r.Speed, port, r.IP)`，再 `url.QueryEscape`。
- 返回 `vless://{uuid}@{host}:{port}?{query}#{escapedAlias}`。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/app/ -run 'TestBuildVlessLink' -v`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/app/vless_link.go internal/app/vless_link_test.go
git commit -m "feat(vless): 生成标准 VLESS 链接"
```

---

### 任务 6：合并与去重

**文件：**
- 修改：`internal/app/vless_link.go`
- 测试：`internal/app/vless_link_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
func TestMergeVlessLines(t *testing.T) {
	existing := []string{
		"vless://u@1.1.1.1:443?a=1#old",
		"# comment",
		"vless://u@2.2.2.2:443?a=1#keep",
	}
	incoming := []string{
		"vless://u@2.2.2.2:443?a=2#new",
		"vless://u@3.3.3.3:443?a=1#add",
	}
	got := mergeVlessLines(existing, incoming)
	want := []string{
		"vless://u@1.1.1.1:443?a=1#old",
		"# comment",
		"vless://u@2.2.2.2:443?a=2#new",
		"vless://u@3.3.3.3:443?a=1#add",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("merge mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestMergeVlessLinesDedupesIncoming(t *testing.T) {
	got := mergeVlessLines(nil, []string{
		"vless://u@1.1.1.1:443#first",
		"vless://u@1.1.1.1:443#second",
	})
	if len(got) != 1 || !strings.HasSuffix(got[0], "#second") {
		t.Fatalf("incoming duplicates should collapse to last: %#v", got)
	}
}

func TestVlessNodeKeyIPv6(t *testing.T) {
	key, ok := vlessNodeKey("vless://u@[2001:db8::1]:443?type=ws#x")
	if !ok || key != "2001:db8::1:443" {
		t.Fatalf("key=%q ok=%v", key, ok)
	}
	if _, ok := vlessNodeKey("# comment"); ok {
		t.Fatal("comment must not parse as a node")
	}
}

func TestRenderVlessFile(t *testing.T) {
	if got := renderVlessFile([]string{"a", "b"}); got != "a\nb\n" {
		t.Fatalf("got %q", got)
	}
	if got := renderVlessFile(nil); got != "" {
		t.Fatalf("empty should be empty, got %q", got)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/app/ -run 'TestMergeVlessLines|TestVlessNodeKeyIPv6|TestRenderVlessFile' -v`
预期：FAIL，`undefined: mergeVlessLines`。

- [ ] **步骤 3：实现合并**

```go
func vlessNodeKey(line string) (string, bool)
func mergeVlessLines(existing, incoming []string) []string
func renderVlessFile(lines []string) string
```

实现要点：
- `vlessNodeKey`：`url.Parse`；`scheme == "vless"`；`Hostname()` 与 `Port()` 都有值；返回 `strings.ToLower(host)+":"+port`。IPv6 的 `Hostname()` 会去掉方括号（Go 的行为），键形如 `2001:db8::1:443`。
- `mergeVlessLines`：先遍历 existing，按 key 去重（保留第一次出现的行）；非 VLESS 行原样保留。再遍历 incoming，重复 key 替换第一次出现的位置；新 key 追加，并记录索引以便同批次内覆盖（同批次重复时后出现者胜出）。
- `renderVlessFile`：`strings.Join(lines, "\n")`，非空时末尾加 `\n`。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/app/ -run 'TestMergeVlessLines|TestVlessNodeKeyIPv6|TestRenderVlessFile' -v`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/app/vless_link.go internal/app/vless_link_test.go
git commit -m "feat(vless): 合并订阅并按 IP:端口 去重"
```

---

### 任务 7：订阅链接接口客户端

**文件：**
- 创建：`internal/app/vless_subscription.go`
- 测试：`internal/app/vless_subscription_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
func TestRequestSubscription(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"url":"https://sub.example/x"}`))
	}))
	defer srv.Close()

	got, err := requestSubscription(context.Background(), srv.URL, "/api/subscription-link", "tok", "vlesssub")
	if err != nil || got != "https://sub.example/x" {
		t.Fatalf("got %q, %v", got, err)
	}
	if gotPath != "/api/subscription-link" || gotAuth != "Bearer tok" || gotBody["file"] != "vlesssub" {
		t.Fatalf("request wrong: path=%q auth=%q body=%+v", gotPath, gotAuth, gotBody)
	}
}

func TestRequestSubscriptionError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"no access"}`))
	}))
	defer srv.Close()
	_, err := requestSubscription(context.Background(), srv.URL, "/api/subscription-link", "tok", "vlesssub")
	if err == nil || !strings.Contains(err.Error(), "no access") {
		t.Fatalf("want error with server message, got %v", err)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/app/ -run 'TestRequestSubscription' -v`
预期：FAIL，`undefined: requestSubscription`。

- [ ] **步骤 3：实现客户端**

```go
func requestSubscription(ctx context.Context, baseURL, apiPath, token, file string) (string, error)
```

实现要点：
- `baseURL` 为空时用 `https://manager.knny.dpdns.org`；`apiPath` 为空时用 `/api/subscription-link`；去尾部 `/`。
- `http.Client{Timeout: 15 * time.Second}`。
- POST JSON `{file}`；`Authorization: Bearer <normalized>`；`Content-Type: application/json`。
- 非 2xx：读取响应体，尝试解析 `{error}`，返回 `fmt.Errorf("获取订阅链接失败 HTTP %d: %s", ...)`。
- 解析 `{url}`；为空返回错误。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/app/ -run 'TestRequestSubscription' -v`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/app/vless_subscription.go internal/app/vless_subscription_test.go
git commit -m "feat(vless): 调用订阅链接接口"
```

---

### 任务 8：回环访问限制

**文件：**
- 创建：`internal/app/vless_server.go`
- 测试：`internal/app/vless_server_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
func TestLocalOnlyMiddleware(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
	handler := localOnly(ok)

	cases := []struct {
		remote string
		want   int
	}{
		{"127.0.0.1:5000", http.StatusOK},
		{"[::1]:5000", http.StatusOK},
		{"192.168.1.5:5000", http.StatusForbidden},
		{"10.0.0.2:5000", http.StatusForbidden},
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", "/api/vless/upload", nil)
		req.RemoteAddr = c.remote
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != c.want {
			t.Errorf("remote %s: got %d want %d", c.remote, rec.Code, c.want)
		}
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/app/ -run TestLocalOnlyMiddleware -v`
预期：FAIL，`undefined: localOnly`。

- [ ] **步骤 3：实现中间件**

```go
func localOnly(next http.HandlerFunc) http.HandlerFunc
func isLoopbackAddr(remote string) bool
```

要点：`net.SplitHostPort` 失败时把整个 `remote` 当 host；`net.ParseIP(host).IsLoopback()`；非回环 `writeErr(w, 403, "VLESS 接口只允许本机访问")`。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/app/ -run TestLocalOnlyMiddleware -v`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/app/vless_server.go internal/app/vless_server_test.go
git commit -m "feat(vless): 限制 VLESS 接口仅本机访问"
```

---

### 任务 9：启用 / 状态 / 刷新接口

**文件：**
- 修改：`internal/app/vless_server.go`
- 修改：`internal/app/server.go:40-53`（路由）
- 测试：`internal/app/vless_server_test.go`

- [ ] **步骤 1：编写失败的测试**

测试需要两个 seam（在 `vless_server.go` 中声明）：

```go
var vlessSaveConfig = SaveConfig
```

测试代码：

```go
func withTestConfig(t *testing.T, c *Config) {
	t.Helper()
	oldCfg := cfg
	cfg = c
	t.Cleanup(func() { cfg = oldCfg })
	oldSave := vlessSaveConfig
	// 模拟 SaveConfig：更新内存里的 cfg，但不落盘
	vlessSaveConfig = func(c *Config) error { cfg = c; return nil }
	t.Cleanup(func() { vlessSaveConfig = oldSave })
}

func TestVlessEnableStatusAndRefresh(t *testing.T) {
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testConfigJS))
	}))
	defer raw.Close()
	oldRaw := rawGithubBase
	rawGithubBase = raw.URL
	defer func() { rawGithubBase = oldRaw }()

	withTestConfig(t, DefaultConfig())
	s := NewServer()

	body := `{"token":"tok","repo":"Owner/NodeWeb","branch":"main","path":"config.js"}`
	req := httptest.NewRequest("POST", "/api/vless/enable", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("enable failed: %d %s", rec.Code, rec.Body.String())
	}
	if !cfg.VlessEnabled || cfg.VlessConfigCache != testConfigJS || cfg.VlessUploadRepo != "Owner/Sub" {
		t.Fatalf("enable did not persist: %+v", cfg)
	}

	statusReq := httptest.NewRequest("GET", "/api/vless/status", nil)
	statusReq.RemoteAddr = "127.0.0.1:5000"
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, statusReq)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"enabled":true`) ||
		!strings.Contains(rec.Body.String(), `"uuid":"uuid-1"`) {
		t.Fatalf("status wrong: %d %s", rec.Code, rec.Body.String())
	}

	refreshReq := httptest.NewRequest("POST", "/api/vless/config/refresh", nil)
	refreshReq.RemoteAddr = "127.0.0.1:5000"
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, refreshReq)
	if rec.Code != 200 {
		t.Fatalf("refresh failed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestVlessEndpointsRejectRemote(t *testing.T) {
	withTestConfig(t, DefaultConfig())
	s := NewServer()
	req := httptest.NewRequest("GET", "/api/vless/status", nil)
	req.RemoteAddr = "192.168.1.9:5000"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("remote status should be 403, got %d", rec.Code)
	}
}

func TestVlessStatusDisabled(t *testing.T) {
	withTestConfig(t, DefaultConfig())
	s := NewServer()
	req := httptest.NewRequest("GET", "/api/vless/status", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/app/ -run 'TestVlessEnableStatusAndRefresh|TestVlessEndpointsRejectRemote' -v`
预期：FAIL，`404` 或 `undefined: vlessSaveConfig`。

- [ ] **步骤 3：实现状态结构与三个接口**

```go
type vlessStatusResponse struct {
	Enabled            bool           `json:"enabled"`
	ConfigRepo         string         `json:"config_repo,omitempty"`
	ConfigBranch       string         `json:"config_branch,omitempty"`
	ConfigPath         string         `json:"config_path,omitempty"`
	HasConfigToken     bool           `json:"has_config_token"`
	UploadRepo         string         `json:"upload_repo,omitempty"`
	HasUploadToken     bool           `json:"has_upload_token"`
	UseConfigForUpload bool           `json:"use_config_for_upload"`
	File               string         `json:"file,omitempty"`
	Count              int            `json:"count"`
	Merge              bool           `json:"merge"`
	ECH                bool           `json:"ech"`
	UUID               string         `json:"uuid,omitempty"`
	Host               string         `json:"host,omitempty"`
	SNI                string         `json:"sni,omitempty"`
	Path               string         `json:"path,omitempty"`
	FP                 string         `json:"fp,omitempty"`
	SubscriptionURL    string         `json:"subscription_url,omitempty"`
	FetchedAt          int64          `json:"fetched_at,omitempty"`
	Defaults           *vlessDefaults `json:"defaults,omitempty"`
	Presets            *vlessPresets  `json:"presets,omitempty"`
	Warning            string         `json:"warning,omitempty"`
}

func (s *Server) vlessStatus(c *Config) vlessStatusResponse
func (s *Server) handleVlessEnable(w http.ResponseWriter, r *http.Request)
func (s *Server) handleVlessStatus(w http.ResponseWriter, r *http.Request)
func (s *Server) handleVlessConfigRefresh(w http.ResponseWriter, r *http.Request)
```

实现要点：
- `vlessStatus`：解析 `c.VlessConfigCache`；解析失败或为空时 `Defaults/Presets` 留空并给出 `Warning`；令牌只回 `Has*`。
- `handleVlessEnable`：只接受 POST；`{token, repo, branch, path}`；缺省用 `c.VlessConfigRepo/Branch/Path` 或默认值；`normalizeGitHubToken`；`fetchNodeWebConfig`；`parseNodeWebConfig`；成功后写 `VlessEnabled=true`、token、来源、cache、fetchedAt；`VlessUploadRepo` 为空时写 `worker.repo`（空则 `github.repo`）；`vlessSaveConfig` 持久化；返回 status。失败时 400/502 并带原因。
- `handleVlessStatus`：返回 `vlessStatus`。
- `handleVlessConfigRefresh`：未启用返回 400；用已保存的 token 重拉；更新 cache/fetchedAt；返回 status。

- [ ] **步骤 4：在 `server.go` 注册路由**

```go
s.mux.HandleFunc("/api/vless/enable", localOnly(s.handleVlessEnable))
s.mux.HandleFunc("/api/vless/status", localOnly(s.handleVlessStatus))
s.mux.HandleFunc("/api/vless/config/refresh", localOnly(s.handleVlessConfigRefresh))
```

（upload / subscription 在任务 10 注册。）

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./internal/app/ -run 'TestVlessEnableStatusAndRefresh|TestVlessEndpointsRejectRemote' -v`
预期：PASS。

- [ ] **步骤 6：Commit**

```bash
git add internal/app/server.go internal/app/vless_server.go internal/app/vless_server_test.go
git commit -m "feat(vless): 启用、状态与配置刷新接口"
```

---

### 任务 10：上报接口与订阅重试

**文件：**
- 修改：`internal/app/vless_server.go`
- 修改：`internal/app/server.go`（路由）
- 测试：`internal/app/vless_server_test.go`

- [ ] **步骤 1：编写失败的测试**

准备一个辅助函数：把 `githubAPIBase` 指向一个记录 GET/PUT 的 httptest，返回预先设置的远端文件内容（默认 404）。

```go
type ghMock struct {
	existing string // 远端已有内容，空表示 404
	put      string // 捕获最后一次 PUT 的 content
	putPath  string
}

func newGHMock(t *testing.T) (*ghMock, func()) {
	m := &ghMock{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if m.existing == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"content": base64.StdEncoding.EncodeToString([]byte(m.existing)),
				"sha":     "sha1", "encoding": "base64",
			})
		case http.MethodPut:
			var body struct{ Content, Message, Branch, Sha string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			raw, _ := base64.StdEncoding.DecodeString(body.Content)
			m.put, m.putPath = string(raw), r.URL.Path
			w.WriteHeader(http.StatusOK)
		}
	}))
	old := githubAPIBase
	githubAPIBase = srv.URL
	return m, func() { srv.Close(); githubAPIBase = old }
}
```

测试用例（先写辅助函数，再写 5 个用例）：

```go
func doVlessUpload(s *Server, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/vless/upload", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

type uploadFixture struct {
	srv   *Server
	gh    *ghMock
	close func()
}

func setupUploadTest(t *testing.T, existing string, rawStatus int, subHandler http.HandlerFunc) *uploadFixture {
	t.Helper()
	sub := httptest.NewServer(subHandler)
	js := strings.Replace(testConfigJS, "https://manager.example", sub.URL, 1)
	raw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rawStatus != http.StatusOK {
			w.WriteHeader(rawStatus)
			return
		}
		_, _ = w.Write([]byte(js))
	}))
	oldRaw := rawGithubBase
	rawGithubBase = raw.URL
	gh, closeGH := newGHMock(t)
	gh.existing = existing

	c := DefaultConfig()
	c.VlessEnabled = true
	c.VlessConfigToken = "cfg-token"
	c.VlessConfigCache = js
	c.VlessUploadRepo = "Owner/Sub"
	c.VlessUploadToken = "up-token"
	c.VlessFile = "vlesssub"
	c.VlessMerge = true
	withTestConfig(t, c)

	s := NewServer()
	s.runner.results = []Result{
		{IP: "1.1.1.1", Port: 443, Speed: 20, SpeedQualified: true, SpeedTested: true, Colo: "HKG"},
		{IP: "2.2.2.2", Port: 443, Speed: 5, SpeedQualified: true, SpeedTested: true, Colo: "SIN"},
		{IP: "3.3.3.3", Port: 443, Speed: 1, SpeedQualified: false, SpeedTested: true},
	}
	return &uploadFixture{srv: s, gh: gh, close: func() {
		closeGH()
		raw.Close()
		sub.Close()
		rawGithubBase = oldRaw
	}}
}

func TestVlessUploadNewFile(t *testing.T) {
	f := setupUploadTest(t, "", http.StatusOK, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"https://sub.example/vlesssub"}`))
	})
	defer f.close()

	rec := doVlessUpload(f.srv, `{"count":0,"merge":true}`)
	if rec.Code != 200 {
		t.Fatalf("upload failed: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"url":"https://sub.example/vlesssub"`) {
		t.Fatalf("url missing: %s", rec.Body.String())
	}
	if f.gh.putPath != "/repos/Owner/Sub/contents/vlesssub" {
		t.Fatalf("wrong put path: %s", f.gh.putPath)
	}
	if strings.Count(f.gh.put, "vless://") != 2 ||
		!strings.Contains(f.gh.put, "@1.1.1.1:443") ||
		!strings.Contains(f.gh.put, "@2.2.2.2:443") {
		t.Fatalf("expected 2 qualified nodes, got:
%s", f.gh.put)
	}
}

func TestVlessUploadMergesExisting(t *testing.T) {
	existing := "vless://u@1.1.1.1:443?old#old
vless://u@9.9.9.9:443?keep#keep
"
	f := setupUploadTest(t, existing, http.StatusOK, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"https://sub.example/x"}`))
	})
	defer f.close()

	rec := doVlessUpload(f.srv, `{"count":0,"merge":true}`)
	if rec.Code != 200 {
		t.Fatalf("upload failed: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Count(f.gh.put, "@1.1.1.1:443") != 1 || strings.Contains(f.gh.put, "#old") {
		t.Fatalf("existing node should be replaced in place:
%s", f.gh.put)
	}
	if !strings.Contains(f.gh.put, "9.9.9.9") || !strings.Contains(f.gh.put, "2.2.2.2") {
		t.Fatalf("merge lost nodes:
%s", f.gh.put)
	}
	if strings.Count(f.gh.put, "vless://") != 3 {
		t.Fatalf("expected 3 unique nodes:
%s", f.gh.put)
	}
}

func TestVlessUploadRejectsEmptyResults(t *testing.T) {
	f := setupUploadTest(t, "", http.StatusOK, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"https://sub.example/x"}`))
	})
	defer f.close()
	f.srv.runner.results = []Result{{IP: "3.3.3.3", Port: 443, Speed: 1, SpeedTested: true, SpeedQualified: false}}

	rec := doVlessUpload(f.srv, `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d %s", rec.Code, rec.Body.String())
	}
	if f.gh.put != "" {
		t.Fatalf("must not PUT when there are no qualified nodes:
%s", f.gh.put)
	}
}

func TestVlessUploadUsesCacheOnFetchFailure(t *testing.T) {
	f := setupUploadTest(t, "", http.StatusInternalServerError, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"https://sub.example/x"}`))
	})
	defer f.close()

	rec := doVlessUpload(f.srv, `{"count":0,"merge":true}`)
	if rec.Code != 200 {
		t.Fatalf("cache fallback failed: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"warning"`) || f.gh.put == "" {
		t.Fatalf("expected warning + upload, got %s
put=%q", rec.Body.String(), f.gh.put)
	}
}

func TestVlessUploadSubscriptionFailure(t *testing.T) {
	f := setupUploadTest(t, "", http.StatusOK, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"manager down"}`))
	})
	defer f.close()

	rec := doVlessUpload(f.srv, `{"count":0,"merge":true}`)
	if rec.Code != 200 {
		t.Fatalf("upload should succeed: %d %s", rec.Code, rec.Body.String())
	}
	if f.gh.put == "" {
		t.Fatal("upload content missing")
	}
	if !strings.Contains(rec.Body.String(), `"subscription_error"`) || strings.Contains(rec.Body.String(), `"url":"`) {
		t.Fatalf("subscription failure must be reported separately: %s", rec.Body.String())
	}
}

func TestVlessSubscriptionEndpoint(t *testing.T) {
	f := setupUploadTest(t, "", http.StatusOK, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"url":"https://sub.example/retry"}`))
	})
	defer f.close()

	req := httptest.NewRequest("POST", "/api/vless/subscription", strings.NewReader(`{"file":"vlesssub"}`))
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.srv.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"url":"https://sub.example/retry"`) {
		t.Fatalf("subscription retry failed: %d %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/app/ -run 'TestVlessUpload' -v`
预期：FAIL，404 或 `undefined: vlessUploadResponse`。

- [ ] **步骤 3：实现上报与订阅接口**

```go
type vlessUploadRequest struct {
	Count              *int   `json:"count"`   // nil = 用已保存的值；0 = 全部
	File               string `json:"file"`
	Merge              *bool  `json:"merge"`   // nil = 用已保存的值
	ECH                *bool  `json:"ech"`
	UUID               string `json:"uuid"`
	Host               string `json:"host"`
	SNI                string `json:"sni"`
	Path               string `json:"path"`
	FP                 string `json:"fp"`
	UploadRepo         string `json:"upload_repo"`
	UploadToken        string `json:"upload_token"`
	UseConfigForUpload bool   `json:"use_config_for_upload"`
}

type vlessUploadResponse struct {
	OK                bool   `json:"ok"`
	Count             int    `json:"count"`
	File              string `json:"file"`
	URL               string `json:"url,omitempty"`
	Merged            bool   `json:"merged"`
	Warning           string `json:"warning,omitempty"`
	SubscriptionError string `json:"subscription_error,omitempty"`
}

func (s *Server) vlessNodes(limit int) ([]Result, error)
func (s *Server) vlessTarget(c *Config, cfg *nodeWebConfig) (repo, token, branch string, err error)
func (s *Server) handleVlessUpload(w http.ResponseWriter, r *http.Request)
func (s *Server) handleVlessSubscription(w http.ResponseWriter, r *http.Request)
```

实现要点：
- `vlessNodes`：`s.runner.Results()`；空则 `ReadCSV(ResultFile)`；`filterSpeedQualified`；按速度降序；`limit>0` 截断；空返回错误。
- 上传参数：请求里的 `count`/`merge`/`ech` 为指针，`nil` 表示沿用已保存配置，`count=0` 表示全部；`file` 为空用 `c.VlessFile`；其余字符串为空时依次回退到已保存配置、解析出的默认值。
- 拉取最新 config：`fetchNodeWebConfig`，失败用 `c.VlessConfigCache` 解析并设置 `warning`；若缓存也没有，返回 502。
- 目标仓库：`use_config_for_upload` → 配置仓库/令牌；否则 `upload_repo`（空则 `worker.repo`/`github.repo`）+ `upload_token`（空则已保存）。校验 `owner/repo` 与令牌非空。
- 分支：解析出的 `Github.Branch`，空则 `main`。
- 生成链接：`vlessOptionsFrom`（config 默认 + 配置覆盖 + 请求覆盖）→ `buildVlessLink` 逐条；任何一条报错则整体失败并返回 400。
- 合并：`merge` 时 `githubGetFile` 读取（404 视为空），`mergeVlessLines`；否则只用新行。`renderVlessFile`。
- PUT：`githubPutFile`，message 为 `fmt.Sprintf("更新 VLESS 订阅 (%d 个) %s", len(lines), time.Now().Format("2006-01-02 15:04"))`。
- 订阅：`requestSubscription`；失败设置 `SubscriptionError`，不覆盖上传成功。
- 仓库不一致 warning：`cfg.Github.Worker.Repo` 非空且与上报仓库不相等（忽略大小写）时设置。
- 保存配置：file/count/merge/ech/overrides/upload repo/useConfig/upload token（仅当非空）/subscription URL。
- `handleVlessSubscription`：`POST {file}`；用保存的配置解析目标令牌；调用 `requestSubscription`；返回 `{url}`。

- [ ] **步骤 4：注册路由**

```go
s.mux.HandleFunc("/api/vless/upload", localOnly(s.handleVlessUpload))
s.mux.HandleFunc("/api/vless/subscription", localOnly(s.handleVlessSubscription))
```

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./internal/app/ -run 'TestVlessUpload|TestVlessSubscription' -v`
预期：PASS。

- [ ] **步骤 6：Commit**

```bash
git add internal/app/server.go internal/app/vless_server.go internal/app/vless_server_test.go
git commit -m "feat(vless): 生成并上报 VLESS 订阅，支持合并与订阅重试"
```

---

### 任务 11：隐藏入口与启用 UI

**文件：**
- 修改：`internal/app/web/index.html`（顶栏、配置弹窗、脚本引入）
- 创建：`internal/app/web/vless.js`
- 修改：`internal/app/web/style.css`

- [ ] **步骤 1：在 `index.html` 的顶栏加隐藏图标**

放在 `#btnTheme` 前：

```html
<button class="btn ghost hidden" id="btnVless" type="button" title="VLESS 订阅" aria-label="VLESS 订阅">
  <svg viewBox="0 0 24 24" class="ico"><circle cx="8" cy="15" r="4"/><path d="M10.8 12.2 20 3m-3 3 3 3m-6 0 3 3"/></svg>
</button>
```

- [ ] **步骤 2：在 `#mask` 的 `h3` 下加隐藏开发区块**

```html
<h3>上报配置</h3>
<div id="vlessDev" class="hidden">
  <hr>
  <em class="note">开发者选项</em>
  <div class="field">
    <label>NodeWeb 令牌</label>
    <input type="password" id="vlessEnableToken" placeholder="github_pat_… 或 ghp_…">
  </div>
  <div class="grid2">
    <div class="field"><label>配置仓库</label><input type="text" id="vlessCfgRepo" value="Daizhouhui/NodeWeb"></div>
    <div class="field"><label>分支</label><input type="text" id="vlessCfgBranch" value="main"></div>
  </div>
  <div class="field"><label>配置文件路径</label><input type="text" id="vlessCfgPath" value="config.js"></div>
  <div class="modal-actions"><button class="btn primary" id="btnVlessEnable">启用</button></div>
</div>
```

- [ ] **步骤 3：在 `</body>` 前引入脚本**

```html
<script src="/app.js"></script>
<script src="/vless.js"></script>
```

- [ ] **步骤 4：实现 `vless.js` 的启用部分**

```js
'use strict';

(function () {
  const $ = (s) => document.querySelector(s);
  const api = window.api;   // app.js 里的函数声明会挂到 window
  const toast = window.toast;

  function isLocalPage() {
    const h = location.hostname;
    return h === '127.0.0.1' || h === 'localhost' || h === '::1';
  }

  let devClicks = 0;
  let devTimer = 0;

  async function refreshStatus() {
    if (!isLocalPage()) return null;
    try {
      const st = await api('/api/vless/status');
      const on = !!(st && st.enabled);
      $('#btnVless').classList.toggle('hidden', !on);
      return st;
    } catch (_) {
      return null;
    }
  }

  function bindDevGesture() {
    const title = document.querySelector('#mask .modal h3');
    if (!title) return;
    title.addEventListener('click', () => {
      if (!isLocalPage()) return;
      devClicks++;
      clearTimeout(devTimer);
      devTimer = setTimeout(() => { devClicks = 0; }, 3000);
      if (devClicks >= 7) {
        devClicks = 0;
        $('#vlessDev').classList.remove('hidden');
      }
    });
  }

  async function enable() {
    const token = $('#vlessEnableToken').value.trim();
    if (!token) { toast('请填写 NodeWeb 令牌', 'err'); return; }
    try {
      const st = await api('/api/vless/enable', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          token,
          repo: $('#vlessCfgRepo').value.trim(),
          branch: $('#vlessCfgBranch').value.trim(),
          path: $('#vlessCfgPath').value.trim(),
        }),
      });
      $('#vlessEnableToken').value = '';
      toast('VLESS 功能已启用', 'ok');
      await refreshStatus();
      if (window.VlessUI) window.VlessUI.open(st);
    } catch (e) {
      toast(e.message, 'err');
    }
  }

  window.VlessUI = { refreshStatus, open: () => {} };

  async function init() {
    bindDevGesture();
    $('#btnVless').addEventListener('click', () => window.VlessUI.open());
    $('#btnVlessEnable').addEventListener('click', enable);
    await refreshStatus();
  }

  document.addEventListener('DOMContentLoaded', init);
})();
```

注意：`$`、`api`、`toast` 来自 `app.js` 的全局声明；`app.js` 在 `vless.js` 之前加载。

- [ ] **步骤 5：手工验证**

运行：`go run ./cmd/yx web -listen 127.0.0.1:18080 -no-open`
用 Playwright 打开 `http://127.0.0.1:18080/`：
1. 点顶栏齿轮打开「上报配置」；连点标题 7 次 → `#vlessDev` 出现（`classList` 不含 `hidden`）。
2. 刷新页面 → `#vlessDev` 再次隐藏，顶栏无 `#btnVless`（未启用时）。
3. 用 Playwright 的 `browser_evaluate` 把 `/api/vless/status` 的 fetch 结果临时改为 `{enabled:true}` 后重载，确认 `#btnVless` 显示（只验证 UI，不需要真实令牌）。

- [ ] **步骤 6：Commit**

```bash
git add internal/app/web/index.html internal/app/web/vless.js internal/app/web/style.css
git commit -m "feat(vless): 隐藏启用入口与顶栏图标"
```

---

### 任务 12：VLESS 面板与上报 UI

**文件：**
- 修改：`internal/app/web/index.html`（新增 `#vlessMask`）
- 修改：`internal/app/web/vless.js`
- 修改：`internal/app/web/style.css`

- [ ] **步骤 1：新增 VLESS 弹窗**

放在 `#cronMask` 之后：

```html
<div class="mask hidden" id="vlessMask">
  <div class="modal">
    <h3>VLESS 订阅</h3>
    <em class="note" id="vlessMeta"></em>

    <div class="field">
      <label>连接参数</label>
      <div class="grid2">
        <div class="field"><span class="hint">UUID</span><input type="text" id="vlessUUID" list="vlessUUIDList"><datalist id="vlessUUIDList"></datalist></div>
        <div class="field"><span class="hint">Host</span><input type="text" id="vlessHost" list="vlessHostList"><datalist id="vlessHostList"></datalist></div>
        <div class="field"><span class="hint">SNI</span><input type="text" id="vlessSNI" list="vlessSNIList"><datalist id="vlessSNIList"></datalist></div>
        <div class="field"><span class="hint">Path</span><input type="text" id="vlessPath" list="vlessPathList"><datalist id="vlessPathList"></datalist></div>
        <div class="field"><span class="hint">Fingerprint</span><input type="text" id="vlessFP" list="vlessFPList"><datalist id="vlessFPList"></datalist></div>
      </div>
      <label class="check"><input type="checkbox" id="vlessECH"> 启用 ECH（默认关闭）</label>
    </div>

    <hr>
    <label class="check"><input type="checkbox" id="vlessUseConfig"> 上报沿用配置仓库和令牌</label>
    <div class="grid2">
      <div class="field"><label>上报仓库</label><input type="text" id="vlessUploadRepo" placeholder="owner/repo"></div>
      <div class="field"><label>上报令牌</label><input type="password" id="vlessUploadToken" placeholder="留空沿用已保存的"></div>
    </div>
    <div class="grid2">
      <div class="field"><label>文件名</label><input type="text" id="vlessFile" value="vlesssub"></div>
      <div class="field"><label>上报数量</label><input type="number" id="vlessCount" min="0" placeholder="留空全部"></div>
    </div>
    <label class="check"><input type="checkbox" id="vlessMerge" checked> 合并到已有订阅（按 IP:端口 去重）</label>

    <div class="modal-actions">
      <button class="btn ghost" id="vlessRefresh">刷新配置</button>
      <span class="spacer"></span>
      <button class="btn ghost" id="vlessClose">关闭</button>
      <button class="btn primary" id="vlessRun">生成并上报</button>
    </div>
    <em class="note" id="vlessResult"></em>
  </div>
</div>
```

- [ ] **步骤 2：实现 `vless.js` 的面板逻辑**

需要实现：
- `VlessUI.open(status?)`：填充表单（status 缺省时先请求 `/api/vless/status`）；`defaults` 与 `presets` 填充 datalist；`file/count/merge/ech/overrides/uploadRepo/useConfig/subscriptionUrl` 回填；打开 `#vlessMask`。
- `VlessUI.open` 在状态为空时显示错误。
- 「刷新配置」：`POST /api/vless/config/refresh`，成功后重新 `open` 填充。
- 「生成并上报」：收集字段 → `POST /api/vless/upload` → 显示 `已上报 N 个节点`；有 `url` 时显示订阅链接和「复制」按钮；有 `warning` / `subscription_error` 时用 `toast` 或结果区显示。
- `useConfig` 勾选时禁用上报仓库/令牌输入。
- 结果区用 `#vlessResult` 显示纯文本；链接用 `a` 元素（`href`、`target="_blank"`、`rel="noreferrer"`）加复制按钮。
- 空数量 → 发送 `count: 0`；文件名空 → `vlesssub`。

- [ ] **步骤 3：补样式**

在 `style.css` 末尾加：

```css
#vlessResult{display:block;margin-top:6px;word-break:break-all}
#vlessResult a{color:var(--accent)}
```

- [ ] **步骤 4：手工验证**

运行：`go run ./cmd/yx web -listen 127.0.0.1:18080 -no-open`
Playwright：
1. 启用后顶栏出现钥匙图标；点击打开 VLESS 面板。
2. 面板里的 UUID/Host/SNI/Path/FP 显示默认值，datalist 有候选。
3. 勾选「上报沿用配置仓库和令牌」→ 上报仓库/令牌输入禁用。
4. 在 390px 视口下，弹窗可滚动、字段不溢出。
5. 用 `browser_evaluate` mock `/api/vless/upload` 返回 `{ok:true,count:3,url:"https://x"}`，点击按钮，结果区显示 3 个节点和链接。

- [ ] **步骤 5：Commit**

```bash
git add internal/app/web/index.html internal/app/web/vless.js internal/app/web/style.css
git commit -m "feat(vless): VLESS 参数面板与上报交互"
```

---

### 任务 13：端到端验证与收尾

**文件：**
- 修改：必要时修正上面任务的文件。

- [ ] **步骤 1：全量 Go 检查**

运行：`go vet ./... && go build ./... && go test ./... -skip TestWritableDir`
预期：全部通过。

- [ ] **步骤 2：跨平台编译检查**

运行：`GOOS=linux GOARCH=arm64 go build ./... && GOOS=windows GOARCH=amd64 go build ./...`
预期：均成功（确认 goja 没有平台限制）。

- [ ] **步骤 3：前端语法检查**

运行：`node --check internal/app/web/app.js && node --check internal/app/web/vless.js`
预期：无输出（通过）。

- [ ] **步骤 4：手工回归现有功能**

1. 原有 Worker 上报按钮仍能打开配置弹窗、保存、上报。
2. 原有 GitHub 上报仍使用 `cloudflare_ips.txt` 格式与原有字段。
3. 测速、筛选下载、CSV 导出、定时任务不受影响。
4. VLESS 未启用时，顶栏没有钥匙图标，配置弹窗没有开发区块。

- [ ] **步骤 5：Commit 收尾**

```bash
git add -A
git commit -m "chore(vless): 端到端验证与收尾"
```

---

## 执行方式

计划完成后请选择：

- **子代理驱动**（推荐）：每个任务由一个全新子代理实现，并在下一个任务前由全新审查者检查；适合这个多文件、跨前后端的功能。
- **原生执行**：在当前会话里逐任务实现，最后做一次整分支审查。
