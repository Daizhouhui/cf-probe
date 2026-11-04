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
	got, err := parseCSVText("IP 地址,下载速度(MB/s),端口\n1.1.1.1,0.00,443\n1.1.1.2,8.00,443\n")
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
		{IP: "1.1.1.1", Port: 443, Speed: 1, SpeedTested: true, SpeedQualified: false},
		{IP: "1.1.1.2", Port: 443, Speed: 5, SpeedTested: true, SpeedQualified: true},
		{IP: "1.1.1.3", Port: 443, Speed: 0, SpeedTested: false, SpeedQualified: false},
	}
	req := httptest.NewRequest("GET", "/api/download?kind=result", nil)
	res := httptest.NewRecorder()
	s.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("want status 200, got %d: %s", res.Code, res.Body.String())
	}
	csv := strings.TrimPrefix(res.Body.String(), "\ufeff")
	got, err := parseCSVText(csv)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].IP != "1.1.1.2" || !got[0].SpeedQualified {
		t.Fatalf("download should contain only speed-qualified nodes, got %+v", got)
	}
}
