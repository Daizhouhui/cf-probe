package app

import (
	"net"
	"testing"

	"github.com/DaiZhouHui/cf-probe/internal/speedtest/utils"
)

func TestAllDelayQualifiedResultsIncludeUntestedCandidates(t *testing.T) {
	candidates := utils.PingDelaySet{
		{PingData: &utils.PingData{IP: &net.IPAddr{IP: net.ParseIP("1.1.1.1")}, Delay: 10}},
		{PingData: &utils.PingData{IP: &net.IPAddr{IP: net.ParseIP("1.0.0.1")}, Delay: 20}, DownloadSpeed: 2 * 1024 * 1024, DownloadTested: true},
		{PingData: &utils.PingData{IP: &net.IPAddr{IP: net.ParseIP("1.0.0.2")}, Delay: 30}, DownloadTested: true},
	}

	results := allDelayQualifiedResults(candidates, 0)
	if len(results) != len(candidates) {
		t.Fatalf("应保留全部延迟符合的候选: want %d, got %d", len(candidates), len(results))
	}
	byIP := make(map[string]Result, len(results))
	for _, result := range results {
		byIP[result.IP] = result
	}
	if result := byIP["1.0.0.1"]; result.Speed != 2 || !result.SpeedTested || !result.SpeedQualified {
		t.Errorf("有正速度的已测速候选应符合零速度下限: got %+v", result)
	}
	if result := byIP["1.1.1.1"]; result.SpeedTested || result.SpeedQualified {
		t.Errorf("未测速候选不应标记为速度符合: got %+v", result)
	}
	if result := byIP["1.0.0.2"]; !result.SpeedTested || result.SpeedQualified {
		t.Errorf("零速度的已测速候选不应标记为速度符合: got %+v", result)
	}
}
