package app

import (
	"net"
	"net/http"
)

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
