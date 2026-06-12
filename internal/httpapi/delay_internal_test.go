package httpapi

import (
	"testing"
	"time"
)

func TestClampDelay(t *testing.T) {
	tests := []struct {
		name    string
		seconds float64
		want    time.Duration
	}{
		{"zero", 0, 0},
		{"fractional", 0.25, 250 * time.Millisecond},
		{"under cap", 5, 5 * time.Second},
		{"at cap", 10, maxDelay},
		{"over cap clamps", 100, maxDelay},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampDelay(tt.seconds); got != tt.want {
				t.Errorf("clampDelay(%v) = %v, want %v", tt.seconds, got, tt.want)
			}
		})
	}
}
