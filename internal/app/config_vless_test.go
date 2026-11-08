package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigHasVlessDefaults(t *testing.T) {
	c := DefaultConfig()
	if c.VlessConfigRepo != "Daizhouhui/NodeWeb" || c.VlessConfigBranch != "main" || c.VlessConfigPath != "config.js" {
		t.Fatalf("config source defaults wrong: %+v", c)
	}
	if c.VlessFile != "vlesssub" || !c.VlessMerge || c.VlessECH {
		t.Fatalf("vless upload defaults wrong: file=%q merge=%v ech=%v", c.VlessFile, c.VlessMerge, c.VlessECH)
	}
	if !c.VlessUseConfigToken {
		t.Fatal("vless should reuse the config token by default")
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
	if !c.VlessUseConfigToken {
		t.Fatal("old config should inherit the config-token default")
	}
}
