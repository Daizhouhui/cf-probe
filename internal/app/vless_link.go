package app

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type vlessLinkOptions struct {
	UUID, Host, SNI, Path, FP, ECH string
	Encryption, Security, Network  string
	ForcePort443, AllowInsecure    bool
	UseECH                         bool
}

// vlessOptionsFrom 用 config 默认值填充，并用 overrides 里的非空值覆盖。
func vlessOptionsFrom(def vlessDefaults, overrides vlessLinkOptions) vlessLinkOptions {
	o := vlessLinkOptions{
		UUID:          def.UUID,
		Host:          def.Host,
		SNI:           def.SNI,
		Path:          def.Path,
		FP:            def.FP,
		ECH:           def.ECH,
		Encryption:    def.Encryption,
		Security:      def.Security,
		Network:       def.Network,
		ForcePort443:  def.ForcePort443,
		AllowInsecure: def.AllowInsecure,
		UseECH:        overrides.UseECH,
	}
	if v := strings.TrimSpace(overrides.UUID); v != "" {
		o.UUID = v
	}
	if v := strings.TrimSpace(overrides.Host); v != "" {
		o.Host = v
	}
	if v := strings.TrimSpace(overrides.SNI); v != "" {
		o.SNI = v
	}
	if v := strings.TrimSpace(overrides.Path); v != "" {
		o.Path = v
	}
	if v := strings.TrimSpace(overrides.FP); v != "" {
		o.FP = v
	}
	if v := strings.TrimSpace(overrides.ECH); v != "" {
		o.ECH = v
	}
	if v := strings.TrimSpace(overrides.Encryption); v != "" {
		o.Encryption = v
	}
	if v := strings.TrimSpace(overrides.Security); v != "" {
		o.Security = v
	}
	if v := strings.TrimSpace(overrides.Network); v != "" {
		o.Network = v
	}
	return o
}

// buildVlessLink 生成一行 VLESS 链接；参数顺序固定，便于和 NodeWeb 的输出对齐。
func buildVlessLink(o vlessLinkOptions, r Result) (string, error) {
	uuid := strings.TrimSpace(o.UUID)
	host := strings.TrimSpace(o.Host)
	sni := strings.TrimSpace(o.SNI)
	if uuid == "" || host == "" || sni == "" {
		return "", fmt.Errorf("VLESS 参数缺少 uuid/host/sni，请先刷新配置")
	}
	enc := o.Encryption
	if enc == "" {
		enc = "none"
	}
	sec := o.Security
	if sec == "" {
		sec = "tls"
	}
	network := o.Network
	if network == "" {
		network = "ws"
	}

	port := vlessPort(o, r)
	hostPart := r.IP
	if strings.Contains(r.IP, ":") {
		hostPart = "[" + r.IP + "]"
	}

	params := []string{
		"encryption=" + url.QueryEscape(enc),
		"security=" + url.QueryEscape(sec),
		"sni=" + url.QueryEscape(sni),
		"fp=" + url.QueryEscape(o.FP),
		"type=" + url.QueryEscape(network),
		"host=" + url.QueryEscape(host),
		"path=" + url.QueryEscape(o.Path),
	}
	if o.UseECH {
		if strings.TrimSpace(o.ECH) == "" {
			return "", fmt.Errorf("已启用 ECH，但 config.js 里没有 ech 参数")
		}
		params = append(params, "ech="+url.QueryEscape(o.ECH))
	}
	if o.AllowInsecure {
		params = append(params, "allowInsecure=1")
	}

	alias := vlessAlias(r, port)
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		uuid, hostPart, port, strings.Join(params, "&"), url.QueryEscape(alias)), nil
}

// vlessPort 决定节点在订阅里的端口：默认用测速端口，forcePort443 时统一 443。
func vlessPort(o vlessLinkOptions, r Result) int {
	port := r.Port
	if port <= 0 {
		port = 443
	}
	if o.ForcePort443 {
		port = 443
	}
	return port
}

// vlessAlias 生成节点别名，例如「东京成田-16.12MB/s-443-108.162.195.14」。
func vlessAlias(r Result, port int) string {
	region := ColoName(r.Colo)
	if region == "" || region == "未知" {
		region = "未知地区"
	}
	return fmt.Sprintf("%s-%.2fMB/s-%d-%s", region, r.Speed, port, r.IP)
}

// vlessNodeKey 从一行 VLESS 链接里取出 host:port，用作去重键。
func vlessNodeKey(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", false
	}
	u, err := url.Parse(line)
	if err != nil || u.Scheme != "vless" {
		return "", false
	}
	host, port := u.Hostname(), u.Port()
	if host == "" || port == "" {
		return "", false
	}
	return strings.ToLower(host) + ":" + port, true
}

// mergeVlessLines 把新节点合并进远端已有内容：同 IP:端口 替换原行，新节点追加。
// 非 VLESS 行原样保留；远端已有的重复项只保留第一行。
func mergeVlessLines(existing, incoming []string) []string {
	out := make([]string, 0, len(existing)+len(incoming))
	index := make(map[string]int)
	for _, line := range existing {
		if key, ok := vlessNodeKey(line); ok {
			if _, dup := index[key]; dup {
				continue
			}
			index[key] = len(out)
		}
		out = append(out, line)
	}
	for _, line := range incoming {
		key, ok := vlessNodeKey(line)
		if !ok {
			out = append(out, line)
			continue
		}
		if idx, exists := index[key]; exists {
			out[idx] = line
			continue
		}
		index[key] = len(out)
		out = append(out, line)
	}
	return out
}

func renderVlessFile(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// proxyNodeKey 从「IP:端口#别名」或「IP:端口」里取出 host:port。
func proxyNodeKey(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}
	if i := strings.IndexByte(line, '#'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	i := strings.LastIndexByte(line, ':')
	if i <= 0 || i == len(line)-1 {
		return "", false
	}
	host := strings.Trim(strings.TrimSpace(line[:i]), "[]")
	port := strings.TrimSpace(line[i+1:])
	if host == "" || port == "" {
		return "", false
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "", false
	}
	return strings.ToLower(host) + ":" + port, true
}

// mergeProxyLines 用于 cloudflare_ips.txt：同 IP:端口 替换原行，新节点追加。
func mergeProxyLines(existing, incoming []string) []string {
	out := make([]string, 0, len(existing)+len(incoming))
	index := make(map[string]int)
	for _, line := range existing {
		if key, ok := proxyNodeKey(line); ok {
			if _, dup := index[key]; dup {
				continue
			}
			index[key] = len(out)
		}
		out = append(out, line)
	}
	for _, line := range incoming {
		key, ok := proxyNodeKey(line)
		if !ok {
			out = append(out, line)
			continue
		}
		if idx, exists := index[key]; exists {
			out[idx] = line
			continue
		}
		index[key] = len(out)
		out = append(out, line)
	}
	return out
}
