package app

import (
	"net"
	"testing"

	"github.com/DaiZhouHui/cf-probe/internal/speedtest/utils"
)

func TestAllDelayQualifiedResultsIncludeUntestedCandidates(t *testing.T) {
	candidates := utils.PingDelaySet{
		{PingData: &utils.PingData{IP: &net.IPAddr{IP: net.ParseIP("1.1.1.1")}, Delay: 10}},
		{PingData: &utils.PingData{IP: &net.IPAddr{IP: net.ParseIP("1.0.0.1")}, Delay: 20}, DownloadSpeed: 2 * 1024 * 1024},
	}

	results := allDelayQualifiedResults(candidates)
	if len(results) != len(candidates) {
		t.Fatalf("应保留全部延迟符合的候选: want %d, got %d", len(candidates), len(results))
	}
	if results[0].IP != "1.0.0.1" || results[0].Speed != 2 {
		t.Fatalf("已测速候选应保留速度结果: got %+v", results[0])
	}
	if results[1].IP != "1.1.1.1" || results[1].Speed != 0 {
		t.Fatalf("未测速候选也应保留在结果中: got %+v", results[1])
	}
}
