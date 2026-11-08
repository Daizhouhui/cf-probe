package app

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

// vlessSaveConfig 单独抽出来，测试时可以替换掉磁盘写入。
var vlessSaveConfig = SaveConfig

// isLoopbackAddr 判断请求来源是否是回环地址。
func isLoopbackAddr(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// localOnly 只放行本机请求；VLESS 令牌保存在服务端，远程页面没有登录认证。
func localOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackAddr(r.RemoteAddr) {
			writeErr(w, http.StatusForbidden, "VLESS 接口只允许本机访问")
			return
		}
		next(w, r)
	}
}

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

func (s *Server) vlessStatus(c *Config) vlessStatusResponse {
	if !c.VlessEnabled {
		return vlessStatusResponse{Enabled: false}
	}
	st := vlessStatusResponse{
		Enabled:            true,
		ConfigRepo:         c.VlessConfigRepo,
		ConfigBranch:       c.VlessConfigBranch,
		ConfigPath:         c.VlessConfigPath,
		HasConfigToken:     c.VlessConfigToken != "",
		UploadRepo:         c.VlessUploadRepo,
		HasUploadToken:     c.VlessUploadToken != "",
		UseConfigForUpload: c.VlessUseConfigForUpload,
		File:               c.VlessFile,
		Count:              c.VlessCount,
		Merge:              c.VlessMerge,
		ECH:                c.VlessECH,
		UUID:               c.VlessUUID,
		Host:               c.VlessHost,
		SNI:                c.VlessSNI,
		Path:               c.VlessPath,
		FP:                 c.VlessFP,
		SubscriptionURL:    c.VlessSubscriptionURL,
		FetchedAt:          c.VlessConfigFetchedAt,
	}
	if strings.TrimSpace(c.VlessConfigCache) != "" {
		if cfg, err := parseNodeWebConfig([]byte(c.VlessConfigCache)); err == nil {
			st.Defaults = &cfg.DefaultVless
			st.Presets = &cfg.VlessPresets
			if st.UploadRepo == "" {
				st.UploadRepo = cfg.Github.Worker.Repo
			}
		} else {
			st.Warning = "上一次的 config.js 解析失败: " + err.Error()
		}
	}
	return st
}

func (s *Server) handleVlessEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "请用 POST")
		return
	}
	var in struct {
		Token  string `json:"token"`
		Repo   string `json:"repo"`
		Branch string `json:"branch"`
		Path   string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	token := normalizeGitHubToken(in.Token)
	if token == "" {
		writeErr(w, http.StatusBadRequest, "请填写 NodeWeb 令牌")
		return
	}
	c := LoadConfig()
	repo := strings.TrimSpace(in.Repo)
	if repo == "" {
		repo = c.VlessConfigRepo
	}
	if repo == "" {
		repo = "Daizhouhui/NodeWeb"
	}
	branch := strings.TrimSpace(in.Branch)
	if branch == "" {
		branch = c.VlessConfigBranch
	}
	if branch == "" {
		branch = "main"
	}
	path := strings.TrimSpace(in.Path)
	if path == "" {
		path = c.VlessConfigPath
	}
	if path == "" {
		path = "config.js"
	}

	raw, err := fetchNodeWebConfig(r.Context(), repo, branch, path, token)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	cfg, err := parseNodeWebConfig(raw)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	c.VlessEnabled = true
	c.VlessConfigRepo = repo
	c.VlessConfigBranch = branch
	c.VlessConfigPath = path
	c.VlessConfigToken = token
	c.VlessConfigCache = string(raw)
	c.VlessConfigFetchedAt = time.Now().UnixMilli()
	if c.VlessUploadRepo == "" {
		c.VlessUploadRepo = cfg.Github.Worker.Repo
		if c.VlessUploadRepo == "" {
			c.VlessUploadRepo = cfg.Github.Repo
		}
	}
	if c.VlessFile == "" {
		c.VlessFile = "vlesssub"
	}
	if err := vlessSaveConfig(c); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.vlessStatus(c))
}

func (s *Server) handleVlessStatus(w http.ResponseWriter, r *http.Request) {
	c := LoadConfig()
	writeJSON(w, http.StatusOK, s.vlessStatus(c))
}

func (s *Server) handleVlessConfigRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "请用 POST")
		return
	}
	c := LoadConfig()
	if !c.VlessEnabled {
		writeErr(w, http.StatusBadRequest, "VLESS 功能未启用")
		return
	}
	raw, err := fetchNodeWebConfig(r.Context(), c.VlessConfigRepo, c.VlessConfigBranch, c.VlessConfigPath, c.VlessConfigToken)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if _, err := parseNodeWebConfig(raw); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	c.VlessConfigCache = string(raw)
	c.VlessConfigFetchedAt = time.Now().UnixMilli()
	if err := vlessSaveConfig(c); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.vlessStatus(c))
}

type vlessUploadRequest struct {
	Count              *int   `json:"count"` // nil = 用已保存的值；0 = 全部
	File               string `json:"file"`
	Merge              *bool  `json:"merge"` // nil = 用已保存的值
	ECH                *bool  `json:"ech"`
	UUID               string `json:"uuid"`
	Host               string `json:"host"`
	SNI                string `json:"sni"`
	Path               string `json:"path"`
	FP                 string `json:"fp"`
	UploadRepo         string `json:"upload_repo"`
	UploadToken        string `json:"upload_token"`
	UseConfigForUpload *bool  `json:"use_config_for_upload"`
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

func (s *Server) vlessNodes(limit int) ([]Result, error) {
	rs := s.runner.Results()
	if len(rs) == 0 {
		var err error
		rs, err = ReadCSV(ResultFile)
		if err != nil {
			return nil, fmt.Errorf("没有可用的测速结果")
		}
	}
	rs = filterSpeedQualified(rs)
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].Speed > rs[j].Speed })
	if limit > 0 && limit < len(rs) {
		rs = rs[:limit]
	}
	if len(rs) == 0 {
		return nil, fmt.Errorf("没有符合速度条件的节点")
	}
	return rs, nil
}

func (s *Server) vlessTarget(c *Config, cfg *nodeWebConfig) (repo, token, branch string, err error) {
	branch = "main"
	if cfg != nil && strings.TrimSpace(cfg.Github.Branch) != "" {
		branch = strings.TrimSpace(cfg.Github.Branch)
	}
	if c.VlessUseConfigForUpload {
		repo, token = c.VlessConfigRepo, c.VlessConfigToken
	} else {
		repo = strings.TrimSpace(c.VlessUploadRepo)
		token = strings.TrimSpace(c.VlessUploadToken)
		if repo == "" && cfg != nil {
			repo = cfg.Github.Worker.Repo
			if repo == "" {
				repo = cfg.Github.Repo
			}
		}
	}
	if !validRepo(repo) {
		return "", "", "", fmt.Errorf("上报仓库格式应为 owner/repo")
	}
	if token == "" {
		return "", "", "", fmt.Errorf("请先填写上报仓库的令牌")
	}
	return repo, token, branch, nil
}

func (s *Server) handleVlessUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "请用 POST")
		return
	}
	var req vlessUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	c := LoadConfig()
	if !c.VlessEnabled {
		writeErr(w, http.StatusBadRequest, "VLESS 功能未启用")
		return
	}

	warning := ""
	var cfg *nodeWebConfig
	raw, err := fetchNodeWebConfig(r.Context(), c.VlessConfigRepo, c.VlessConfigBranch, c.VlessConfigPath, c.VlessConfigToken)
	if err == nil {
		cfg, err = parseNodeWebConfig(raw)
	}
	if err != nil || cfg == nil {
		if strings.TrimSpace(c.VlessConfigCache) == "" {
			msg := "拉取 config.js 失败，且没有可用缓存"
			if err != nil {
				msg += ": " + err.Error()
			}
			writeErr(w, http.StatusBadGateway, msg)
			return
		}
		cfg, err = parseNodeWebConfig([]byte(c.VlessConfigCache))
		if err != nil {
			writeErr(w, http.StatusBadGateway, "解析缓存的 config.js 失败: "+err.Error())
			return
		}
		warning = "拉取最新 config.js 失败，已使用上一次缓存"
	}

	if req.UseConfigForUpload != nil {
		c.VlessUseConfigForUpload = *req.UseConfigForUpload
	}
	if v := strings.TrimSpace(req.UploadRepo); v != "" {
		c.VlessUploadRepo = v
	}
	if v := strings.TrimSpace(req.UploadToken); v != "" {
		c.VlessUploadToken = normalizeGitHubToken(v)
	}
	if v := strings.TrimSpace(req.File); v != "" {
		c.VlessFile = v
	}
	if c.VlessFile == "" {
		c.VlessFile = "vlesssub"
	}
	if !validGitPath(c.VlessFile) {
		writeErr(w, http.StatusBadRequest, "文件名不合法")
		return
	}
	if req.Count != nil {
		c.VlessCount = *req.Count
	}
	if c.VlessCount < 0 {
		c.VlessCount = 0
	}
	if req.Merge != nil {
		c.VlessMerge = *req.Merge
	}
	if req.ECH != nil {
		c.VlessECH = *req.ECH
	}
	if v := strings.TrimSpace(req.UUID); v != "" {
		c.VlessUUID = v
	}
	if v := strings.TrimSpace(req.Host); v != "" {
		c.VlessHost = v
	}
	if v := strings.TrimSpace(req.SNI); v != "" {
		c.VlessSNI = v
	}
	if v := strings.TrimSpace(req.Path); v != "" {
		c.VlessPath = v
	}
	if v := strings.TrimSpace(req.FP); v != "" {
		c.VlessFP = v
	}

	opts := vlessOptionsFrom(cfg.DefaultVless, vlessLinkOptions{
		UUID: c.VlessUUID, Host: c.VlessHost, SNI: c.VlessSNI,
		Path: c.VlessPath, FP: c.VlessFP, UseECH: c.VlessECH,
	})
	nodes, err := s.vlessNodes(c.VlessCount)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	lines := make([]string, 0, len(nodes))
	for _, node := range nodes {
		line, err := buildVlessLink(opts, node)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		lines = append(lines, line)
	}

	repo, token, branch, err := s.vlessTarget(c, cfg)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	var existing []byte
	merged := false
	if c.VlessMerge {
		existing, _, err = githubGetFile(r.Context(), repo, branch, c.VlessFile, token)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	finalLines := lines
	if c.VlessMerge {
		var existingLines []string
		if len(existing) > 0 {
			existingLines = strings.Split(strings.ReplaceAll(string(existing), "\r\n", "\n"), "\n")
			for len(existingLines) > 0 && strings.TrimSpace(existingLines[len(existingLines)-1]) == "" {
				existingLines = existingLines[:len(existingLines)-1]
			}
			merged = len(existingLines) > 0
		}
		finalLines = mergeVlessLines(existingLines, lines)
	}
	content := renderVlessFile(finalLines)
	if content == "" {
		writeErr(w, http.StatusBadRequest, "没有可写入的内容")
		return
	}
	msg := fmt.Sprintf("更新 VLESS 订阅 (%d 个) %s", len(lines), time.Now().Format("2006-01-02 15:04"))
	if err := githubPutFile(r.Context(), repo, branch, c.VlessFile, token, msg, []byte(content)); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	subURL, subErr := requestSubscription(r.Context(),
		cfg.Github.Worker.BaseURL, cfg.Github.Worker.SubscriptionAPIPath, token, c.VlessFile)
	resp := vlessUploadResponse{OK: true, Count: len(lines), File: c.VlessFile, Merged: merged, Warning: warning}
	if subErr != nil {
		resp.SubscriptionError = subErr.Error()
	} else {
		resp.URL = subURL
		c.VlessSubscriptionURL = subURL
	}
	if workerRepo := strings.TrimSpace(cfg.Github.Worker.Repo); workerRepo != "" && !strings.EqualFold(repo, workerRepo) {
		if resp.Warning != "" {
			resp.Warning += "；"
		}
		resp.Warning += "上报仓库与 config.js 的 worker.repo 不一致，订阅链接可能不可用"
	}
	if err := vlessSaveConfig(c); err != nil {
		if resp.Warning != "" {
			resp.Warning += "；"
		}
		resp.Warning += "配置保存失败: " + err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleVlessSubscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "请用 POST")
		return
	}
	var in struct {
		File string `json:"file"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	c := LoadConfig()
	if !c.VlessEnabled {
		writeErr(w, http.StatusBadRequest, "VLESS 功能未启用")
		return
	}
	file := strings.TrimSpace(in.File)
	if file == "" {
		file = c.VlessFile
	}
	if !validGitPath(file) {
		writeErr(w, http.StatusBadRequest, "文件名不合法")
		return
	}
	var cfg *nodeWebConfig
	if strings.TrimSpace(c.VlessConfigCache) != "" {
		cfg, _ = parseNodeWebConfig([]byte(c.VlessConfigCache))
	}
	_, token, _, err := s.vlessTarget(c, cfg)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	baseURL, apiPath := "", ""
	if cfg != nil {
		baseURL = cfg.Github.Worker.BaseURL
		apiPath = cfg.Github.Worker.SubscriptionAPIPath
	}
	url, err := requestSubscription(r.Context(), baseURL, apiPath, token, file)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}
