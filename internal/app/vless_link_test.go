package app

import (
	"strings"
	"testing"
)

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

func TestVlessAliasAndProxyLine(t *testing.T) {
	r := Result{IP: "108.162.195.14", Port: 443, Speed: 16.12, Colo: "NRT"}
	alias := vlessAlias(r, 443)
	if alias != "东京成田-16.12MB/s-443-108.162.195.14" {
		t.Fatalf("alias = %q", alias)
	}
	if line := r.IP + ":443#" + alias; line != "108.162.195.14:443#东京成田-16.12MB/s-443-108.162.195.14" {
		t.Fatalf("line = %q", line)
	}
}

func TestMergeProxyLines(t *testing.T) {
	existing := []string{
		"1.1.1.1:443#old",
		"# comment",
		"2.2.2.2:2053#keep",
	}
	incoming := []string{
		"1.1.1.1:443#new",
		"3.3.3.3:443#add",
	}
	got := mergeProxyLines(existing, incoming)
	want := []string{
		"1.1.1.1:443#new",
		"# comment",
		"2.2.2.2:2053#keep",
		"3.3.3.3:443#add",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("merge mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestProxyNodeKeyIPv6(t *testing.T) {
	for _, line := range []string{"[2001:db8::1]:443#x", "2001:db8::1:443#x"} {
		key, ok := proxyNodeKey(line)
		if !ok || key != "2001:db8::1:443" {
			t.Fatalf("%q -> key=%q ok=%v", line, key, ok)
		}
	}
	if _, ok := proxyNodeKey("# comment"); ok {
		t.Fatal("comment must not parse")
	}
}
