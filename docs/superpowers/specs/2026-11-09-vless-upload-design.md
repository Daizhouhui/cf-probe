# 隐藏的 VLESS 订阅上报功能 — 设计文档

日期：2026-11-09
状态：待审阅

## 1. 意图

给现有的 Cloudflare 优选测速工具增加一条独立于原有上报流程的能力：

- 用测速结果生成 VLESS 订阅链接（一行一条），上传到指定 GitHub 仓库；
- 拉取用户 NodeWeb 仓库里的 `config.js`，用其中的默认参数和候选值生成链接；
- 上传后调用订阅管理接口，拿到可用的订阅 URL；
- 入口隐藏，只有拿到 NodeWeb 仓库令牌的人才能启用，启用后长期保持。

原有 Worker / GitHub 上报逻辑、测速逻辑、CSV 导出等一律不改。

## 2. 目标与非目标

### 目标

1. 隐藏启用：在「上报配置」弹窗里连点标题 7 次，出现 NodeWeb 令牌输入；用该令牌成功拉取 `config.js` 后启用，并持久化。
2. 启用后有一个不显眼的小入口（顶栏图标），打开 VLESS 面板。
3. 自动拉取并解析 `Daizhouhui/NodeWeb/main/config.js`，得到默认连接参数与候选值；每次上报前重新拉取，失败时退回上一次缓存。
4. 按测速结果生成标准 VLESS 链接：默认不带 ECH，别名格式 `地区-速度MB/s-端口-IP`。
5. 上传支持两种模式：合并（按 IP:端口 去重）和覆盖；文件名默认 `vlesssub`，数量留空表示全部。
6. 上传成功后调用订阅接口，显示并保存订阅 URL。
7. 所有配置（仓库、令牌、文件名、数量、合并模式、VLESS 参数等）持久化到本地 `yx-config.json`，后续只点一次按钮即可。

### 非目标

- 不解析或生成 Clash / Sing-box 等其他格式。
- 不实现二维码、节点测活、订阅重命名等 NodeWeb 前端已有的功能。
- 不改变现有 Worker / GitHub 上报的字段、格式和入口。
- 不新增账号体系或云端配置同步。

### 反目标（即使能跑通也算失败）

- 让普通用户容易发现、误触或误配置这个入口。
- 改动现有上报行为，或让原有 GitHub Token 配置与 VLESS 配置互相覆盖。
- 把令牌以明文返回给浏览器或写进日志。
- 合并时产生重复的 IP:端口，或丢失远端已有节点。

## 3. 用户已确定的约束

- 启用凭据是 `Daizhouhui/NodeWeb` 仓库的 GitHub 令牌。
- 配置来源是 `https://raw.githubusercontent.com/Daizhouhui/NodeWeb/main/config.js`（RAW 格式，优先直接 GET；失败时退回 GitHub Contents API）。
- 订阅链接接口只对 `config.js` 里 `github.worker.repo` 指定的仓库生效（默认 `DaiZhouHui/CustomNode`），认证令牌就是该仓库的令牌。
- 链接形如：
  `vless://99999999-9999-8999-9999-99abcdefabcd@104.18.28.101:443?encryption=none&security=tls&sni=lin.kyma.dpdns.org&fp=chrome&type=ws&host=lin.kyma.dpdns.org&path=%2F%3Fed%3D2048#%E6%B4%9B%E6%9D%89%E7%9F%B6-1.55MB%2Fs-443-104.18.28.101`
- 默认不开启 ECH。
- 别名示例：`新加坡-23.79MB/s-443-108.162.195.14`。
- 订阅接口：`POST {worker.baseUrl}{worker.subscriptionApiPath}`，Body `{ "file": "<文件名>" }`，`Authorization: Bearer <上报仓库令牌>`，返回 `{ "url": "..." }`。
- 原有功能与逻辑保持不变。

## 4. 建造者自行决定的部分

以下选择由实现者决定，并在实现时按此执行（审阅时可指出不合理处）：

1. **隐藏手势**：连点配置弹窗标题 7 次、3 秒内有效；无任何视觉提示。启用成功后顶栏出现一个小图标（钥匙形状），仅启用后可见。
2. **节点来源**：从当前测速结果（内存结果优先，为空时读 `result.csv`）中取 `SpeedQualified=true` 的节点，按速度从高到低排序；数量留空 = 全部。
3. **VLESS 参数来源**：默认使用 `defaultVlessConfig` 的 uuid/host/sni/path/fp；面板可用 `vlessPresets` 切换，覆盖值保存在本地。端口默认用节点测速端口；若 `forcePort443` 为真则统一 443。`insecure`/`allowInsecure` 为真时追加 `allowInsecure=1`，默认不追加。
4. **config.js 解析**：引入纯 Go 的 `github.com/dop251/goja`（约 2–4MB 体积增量），在隔离 VM 中执行并 `JSON.stringify(window.NODE_WEB_CONFIG)`；带 2 秒超时中断。避免手写 JS 字面量解析器在注释、单引号、尾逗号上出错。
5. **配置拉取**：优先使用 RAW 地址 `https://raw.githubusercontent.com/{repo}/{branch}/{path}` 直接 GET（更快、更容易访问）；失败时再退回 GitHub Contents API（base64）作为兜底。两个地址都用配置里的令牌鉴权。
6. **配置缓存**：把最近一次成功拉取的 `config.js` 原文存入 `vless_config_cache` 与时间戳；每次上报前重拉，失败则用缓存并提示。
7. **上传仓库默认值**：取 `config.js` 的 `github.worker.repo`（默认 `DaiZhouHui/CustomNode`），允许修改；修改后若与 `worker.repo` 不一致，订阅接口大概率失败，界面会明确提示。
8. **合并语义**：读取远端文件 → 按 `IP:端口` 建索引 → 新节点若命中已有键，则**替换该行的内容但保留原位置**；未命中则追加到末尾；非 VLESS 行原样保留。覆盖模式直接用新内容写回。
9. **订阅接口认证**：使用**上报仓库的令牌**（勾选「上报沿用配置仓库」时则为配置仓库令牌）。
10. **新增接口**：`enable` / `status` / `config/refresh` / `upload` / `subscription`，全部在本地 Web 服务内，不引入外部依赖。
11. **访问范围**：所有 `/api/vless/*` 接口只接受回环地址（`127.0.0.0/8`、`::1`），其他来源一律 403。前端在非回环页面（例如 `-listen 0.0.0.0` 后通过局域网访问、Docker 远程访问）不渲染入口。本机与安卓 App 不受影响。
12. **文件路径**：默认仓库根目录下的 `vlesssub`；名称字段允许带子目录（如 `sub/vlesssub`），但传递订阅接口时使用原样字符串。

## 5. 数据模型

在 `internal/app/config.go` 的 `Config` 中追加字段（全部 `omitempty` 或带默认值，保证旧配置文件兼容）：

```go
VlessEnabled            bool   `json:"vless_enabled"`
VlessConfigRepo         string `json:"vless_config_repo"`      // 默认 Daizhouhui/NodeWeb
VlessConfigBranch       string `json:"vless_config_branch"`    // 默认 main
VlessConfigPath         string `json:"vless_config_path"`      // 默认 config.js
VlessConfigToken        string `json:"vless_config_token"`
VlessConfigCache        string `json:"vless_config_cache"`     // 最近一次成功拉取的原文
VlessConfigFetchedAt    int64  `json:"vless_config_fetched_at"`
VlessUseConfigForUpload bool   `json:"vless_use_config_for_upload"`
VlessUploadRepo         string `json:"vless_upload_repo"`      // 默认取 config.js worker.repo
VlessUploadToken        string `json:"vless_upload_token"`
VlessFile               string `json:"vless_file"`             // 默认 vlesssub
VlessCount              int    `json:"vless_count"`            // 0 = 全部
VlessMerge              bool   `json:"vless_merge"`            // 默认 true
VlessECH                bool   `json:"vless_ech"`              // 默认 false
VlessUUID               string `json:"vless_uuid"`             // 空 = 用 config 默认
VlessHost               string `json:"vless_host"`
VlessSNI                string `json:"vless_sni"`
VlessPath               string `json:"vless_path"`
VlessFP                 string `json:"vless_fp"`
VlessSubscriptionURL    string `json:"vless_subscription_url"`
```

`SaveConfig` 仍然整体写 `yx-config.json`（权限 0600）。读取旧文件时这些字段为零值，由默认值补全逻辑处理。

## 6. 配置拉取与解析

### 6.1 拉取

首选 RAW 地址（更快、更容易访问）：

```
GET https://raw.githubusercontent.com/{configRepo}/{branch}/{configPath}
Authorization: Bearer {configToken}
Accept: text/plain
```

- 2xx → 直接得到 JS 文本，限制 2MB。
- 失败（401/403/404/网络错误）→ 再尝试 GitHub Contents API 作为兜底：

```
GET https://api.github.com/repos/{configRepo}/contents/{configPath}?ref={branch}
Authorization: Bearer {configToken}
Accept: application/vnd.github+json
X-GitHub-Api-Version: 2022-11-28
```

返回 JSON 中的 `content` 为 base64（含换行），解码后得到 JS 文本。

- 两种方式都失败时，返回统一的权限/路径提示。

### 6.2 解析

```go
vm := goja.New()
vm.Set("window", vm.NewObject())
// 2 秒超时中断
_, err := vm.RunString(string(js))
raw, err := vm.RunString(`JSON.stringify(window.NODE_WEB_CONFIG)`)
json.Unmarshal([]byte(raw.String()), &nodeWebConfig)
```

需要的结构（只取用到的字段）：

```go
type vlessDefaults struct {
    UUID, Host, SNI, Path, FP, ECH       string
    Port                                 int
    Prefix                               string
    Encryption, Security, Network        string
    Insecure, AllowInsecure              bool
    ForcePort443                         bool
}
type vlessPresets struct {
    UUID, Host, SNI, Path, FP []string
}
type nodeWebConfig struct {
    DefaultVless  vlessDefaults `json:"defaultVlessConfig"`
    Github        struct {
        Repo   string `json:"repo"`
        Branch string `json:"branch"`
        Worker struct {
            BaseURL          string `json:"baseUrl"`
            Repo             string `json:"repo"`
            SubscriptionAPIPath string `json:"subscriptionApiPath"`
        } `json:"worker"`
    } `json:"github"`
    VlessPresets vlessPresets `json:"vlessPresets"`
}
```

解析成功后：缓存原文 + 时间戳；若 `VlessUploadRepo` 为空，写入 `worker.repo`（空则 `github.repo`）。

## 7. 后端接口

所有 VLESS 接口只接受**回环地址**（`RemoteAddr` 为 `127.0.0.0/8` 或 `::1`），其他来源直接返回 403。原因：程序支持 `-listen 0.0.0.0` 和 Docker 部署，页面本身没有登录认证，而 VLESS 令牌保存在服务端。本机与安卓 App 访问 `127.0.0.1`，不受影响；远程/Docker 页面看不到入口，也不能调用这些接口。

所有接口仅本地监听，沿用现有 `writeJSON` / `writeErr` 风格。

### 7.1 `POST /api/vless/enable`

请求：`{token, repo?, branch?, path?}`。
逻辑：规范化令牌 → 拉取 config.js → 解析 → 写入 `VlessEnabled=true` 及来源配置、缓存、默认上报仓库 → 返回摘要。
失败：令牌为空 / 拉取失败 / 解析失败，返回 400 或 502。

### 7.2 `GET /api/vless/status`

未启用：`{enabled:false}`。
已启用：返回
`{enabled:true, configRepo, configBranch, configPath, hasConfigToken, uploadRepo, hasUploadToken, file, count, merge, ech, uuid, host, sni, path, fp, subscriptionUrl, fetchedAt, defaults:{...}, presets:{...}}`。
令牌只返回 `hasXxx` 布尔值。

### 7.3 `POST /api/vless/config/refresh`

重新拉取并解析，更新缓存；返回与 status 相同的摘要。

### 7.4 `POST /api/vless/upload`

请求：`{count, file, merge, ech, uuid, host, sni, path, fp, uploadRepo, uploadToken?, useConfigForUpload}`。
逻辑：

1. 校验已启用。
2. 重新拉取 config.js（失败用缓存，并在响应里带 `warning`）。
3. 取节点：`runner.Results()`，为空则 `ReadCSV(ResultFile)`；`filterSpeedQualified`；按速度降序；按 `count` 截断（0 = 全部）。
4. 生成 VLESS 行。
5. 决定仓库与令牌：
   - `useConfigForUpload=true` → 配置仓库 + 配置令牌；
   - 否则 → `uploadRepo`（默认 `worker.repo`）+ `uploadToken`（留空沿用已保存）。
6. `merge=true` → 读取远端文件（404 视为空），按 IP:端口 合并去重；否则直接覆盖。
7. PUT 文件（带 sha）。
8. 调用订阅接口，拿 `url`；失败不推翻上传结果，返回 `subscription_error`。
9. 保存本次参数、订阅 URL。

响应：`{ok, count, file, url, merged, warning, subscription_error}`。

### 7.5 `POST /api/vless/subscription`

请求：`{file}`。用上报令牌重新调用订阅接口，返回 `{url}`。用于上传成功但取链接失败时重试。

## 8. VLESS 链接生成

```go
func buildVlessLink(cfg vlessDefaults, r Result, ech bool) string
```

- UUID/SNI/Host/Path/FP：面板覆盖值优先，否则 config 默认值。
- 端口：`r.Port`，`forcePort443` 时固定 443；别名里的端口与链接一致。
- IPv6：主机部分写 `[ip]`。
- 查询参数顺序：`encryption`、`security`、`sni`、`fp`、`type`、`host`、`path`；`ech` 开启时追加 `ech`；`allowInsecure` 为真时追加 `allowInsecure=1`。
- 编码：`path`、`ech`、别名都做 `url.QueryEscape`（例如 `/?ed=2048` → `%2F%3Fed%3D2048`，`MB/s` → `MB%2Fs`）。
- 别名：`ColoName(colo)`（空/未知时 `未知地区`）+ `-` + `%.2fMB/s` + `-` + 端口 + `-` + IP。

## 9. 合并与去重

- 读取远端文件文本，按行处理；保留空行和非 VLESS 行。
- 对每个 VLESS 行，用 `net/url` 解析出 `Hostname()` 与 `Port()`，键为 `host:port`（host 统一小写）。
- 新行命中已有键：替换该索引的整行内容（保留位置）；未命中：追加。
- 最终按原顺序输出，末尾补一个换行。
- 覆盖模式：不做读取，直接写新内容。

## 10. 前端

### 10.1 隐藏启用

- 在 `#mask`（上报配置弹窗）的 `<h3>` 上监听点击：3 秒内累计 7 次，显示 `#vlessDev` 区块（NodeWeb 令牌、可选的仓库/分支/文件路径、启用按钮）。
- 无任何样式暗示；不写入任何可见文案。
- 启用成功后：`state.vlessEnabled=true`，显示顶栏 `#btnVless`（小钥匙图标，`title="VLESS"`），并隐藏/保留 dev 区块（建议保留，便于更换令牌）。
- 页面加载时读取 `/api/vless/status`，已启用则显示入口。
- 访问范围判断：前端检查 `location.hostname` 是否为 `127.0.0.1` / `localhost` / `::1`；非回环页面不显示 7 连点入口和顶栏图标。服务端仍会独立校验，前端判断只是避免误导。

### 10.2 VLESS 面板（`#vlessMask`）

字段：

1. 状态行：配置仓库/分支/路径、上次拉取时间、「刷新配置」。
2. 连接参数：uuid / host / sni / path / fp 五个下拉，候选来自 presets；首项显示 config 默认值；ECH 复选框（默认关）。
3. 上报：仓库（默认 worker.repo）、令牌（`type=password`，留空沿用已保存）、文件名（默认 vlesssub）、数量（留空=全部）、「合并到已有订阅（按 IP:端口 去重）」复选框（默认开）。勾选「上报沿用配置仓库和令牌」时隐藏上报仓库/令牌输入。
4. 按钮：「生成并上报」；结果显示：节点数、订阅链接（可复制）、警告/错误。
5. 提示：订阅链接接口只对 `worker.repo` 仓库生效。

移动端复用现有 `.mask/.modal` 布局，字段纵向堆叠，弹窗内滚动。

## 11. 安全

- **访问范围**：`/api/vless/*` 全部只允许回环来源（`127.0.0.0/8`、`::1`），非回环一律 403；前端在非回环页面隐藏入口。
- 所有令牌只存在服务端 `yx-config.json`（0600），接口只回传 `hasToken` 布尔。
- 新接口不接收任意 URL；仓库/分支/路径做基本校验（`owner/repo`、禁止 `..`）。
- 拉取 config.js 的响应大小限制（例如 2MB），goja 执行带超时。
- 日志不打印令牌。

## 12. 测试

1. `config.js` 解析：用 `test/config.js` 的裁剪副本作为 fixture，断言 defaultVlessConfig 与 presets。
2. 链接生成：用用户给出的示例，断言逐字符一致。
3. 合并去重：已有行 + 新行，同 IP:端口 应替换而非重复；顺序保持；非 VLESS 行保留。
4. 令牌规范化与权限提示（已有测试）。
5. GitHub Contents API 的 GET/PUT 用 `httptest` 模拟（API base 做成包级变量便于测试）。
6. 前端用 Playwright 验证：7 连点出现入口、启用流程（mock 接口）、面板打开、合并开关、订阅 URL 展示。

## 13. 风险与已知限制

- 上报仓库与 `worker.repo` 不一致时，订阅接口会失败；界面会明确提示，但仍允许上传。
- RAW 地址对私有仓库需要带令牌；若 raw 不可用会自动退回 Contents API。
- 远程/Docker 页面（非回环访问）无法启用或使用 VLESS；这是有意的安全取舍。
- config.js 结构若大改（例如不再挂 `window.NODE_WEB_CONFIG`），需要更新解析字段。
- goja 会增加二进制体积；Android AAR 会随之变大。
- 订阅接口的 Bearer 令牌必须是上报仓库令牌；若用户填错，会返回 401/403/404 并给出提示。
