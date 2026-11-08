package app

import (
	"encoding/json"
	"net"
	"net/http"
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
