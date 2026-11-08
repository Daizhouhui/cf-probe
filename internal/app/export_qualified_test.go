package app

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQualifiedCSVPreservesSpeedStatus(t *testing.T) {
	want := []Result{
		{IP: "1.1.1.1", Port: 443, Speed: 5.2, SpeedTested: true, SpeedQualified: true},
		{IP: "1.1.1.2", Port: 443, Speed: 1.1, SpeedTested: true, SpeedQualified: false},
		{IP: "1.1.1.3", Port: 443, Speed: 0, SpeedTested: false, SpeedQualified: false},
	}
	data, err := encodeCSV(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseCSVText(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("want %d CSV rows, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i].SpeedTested != want[i].SpeedTested || got[i].SpeedQualified != want[i].SpeedQualified {
			t.Errorf("row %d status lost during CSV round-trip: got tested=%t qualified=%t", i, got[i].SpeedTested, got[i].SpeedQualified)
		}
	}
}

func TestLegacyCSVDoesNotQualifyUnmeasuredRows(t *testing.T) {
	got, err := parseCSVText("IP 地址,下载速度(MB/s),端口\n1.1.1.1,0.00,443\n1.1.1.2,12.00,443\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
	if got[0].SpeedTested || got[0].SpeedQualified {
		t.Fatalf("legacy 0-speed row must not be treated as tested or qualified: %+v", got[0])
	}
	if !got[1].SpeedTested || !got[1].SpeedQualified {
		t.Fatalf("legacy positive-speed row should be qualified using current threshold: %+v", got[1])
	}
}

func TestDownloadCSVContainsOnlySpeedQualifiedResults(t *testing.T) {
	s := NewServer()
	s.runner.results = []Result{
		{IP: "1.1.1.1", Port: 443, Speed: 1, SpeedTested: true, SpeedQualified: false, Colo: "SIN"},
		{IP: "1.1.1.2", Port: 2053, Speed: 5, SpeedTested: true, SpeedQualified: true, Colo: "HKG"},
		{IP: "1.1.1.3", Port: 443, Speed: 0, SpeedTested: false, SpeedQualified: false},
	}
	req := httptest.NewRequest("GET", "/api/download?kind=result", nil)
	res := httptest.NewRecorder()
	s.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("want status 200, got %d: %s", res.Code, res.Body.String())
	}
	// \u4e0b\u8f7d\u6587\u4ef6\u662f\u7ed9\u5916\u90e8\u5ba2\u6237\u7aef\u7528\u7684\u7cbe\u7b80\u683c\u5f0f
	lines := strings.Split(strings.TrimSpace(res.Body.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), res.Body.String())
	}
	if got := strings.TrimSpace(lines[0]); got != "IP 地址,已发送,已接收,丢包率,平均延迟,下载速度(MB/s),地区码,端口" {
		t.Errorf("header = %q, want the legacy 8-column format", got)
	}
	if got := strings.TrimSpace(lines[1]); got != "1.1.1.2,0,0,0.00,0.00,5.00,HKG,2053" {
		t.Errorf("row = %q, want the speed-qualified node only", got)
	}
}
