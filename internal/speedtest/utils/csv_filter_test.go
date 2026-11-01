package utils

import (
	"net"
	"testing"
	"time"
)

func TestPassesFiltersMatchesDelayAndLossRules(t *testing.T) {
	oldMin, oldMax, oldLoss := InputMinDelay, InputMaxDelay, InputMaxLossRate
	defer func() {
		InputMinDelay, InputMaxDelay, InputMaxLossRate = oldMin, oldMax, oldLoss
	}()
	InputMinDelay = 0
	InputMaxDelay = 100 * time.Millisecond
	InputMaxLossRate = 0.25

	tests := []struct {
		name string
		data CloudflareIPData
		want bool
	}{
		{
			name: "matches delay and loss limits",
			data: CloudflareIPData{PingData: &PingData{
				IP: &net.IPAddr{IP: net.ParseIP("1.1.1.1")}, Sended: 4, Received: 4, Delay: 50 * time.Millisecond,
			}},
			want: true,
		},
		{
			name: "delay exceeds limit",
			data: CloudflareIPData{PingData: &PingData{
				IP: &net.IPAddr{IP: net.ParseIP("1.1.1.1")}, Sended: 4, Received: 4, Delay: 101 * time.Millisecond,
			}},
			want: false,
		},
		{
			name: "loss exceeds limit",
			data: CloudflareIPData{PingData: &PingData{
				IP: &net.IPAddr{IP: net.ParseIP("1.1.1.1")}, Sended: 4, Received: 2, Delay: 50 * time.Millisecond,
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PassesFilters(tt.data); got != tt.want {
				t.Fatalf("PassesFilters() = %v, want %v", got, tt.want)
			}
		})
	}
}
