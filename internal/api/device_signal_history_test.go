package api

import (
	"testing"
	"time"

	"github.com/boa-z/vohive/internal/config"
	"github.com/boa-z/vohive/internal/device"
)

func TestResolveSignalHistoryRange(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name         string
		span, bucket time.Duration
	}{
		{"day", 24 * time.Hour, 5 * time.Minute},
		{"week", 7 * 24 * time.Hour, 30 * time.Minute},
		{"month", 30 * 24 * time.Hour, 2 * time.Hour},
	} {
		name, since, bucket, err := resolveSignalHistoryRange(test.name, now, 30)
		if err != nil || name != test.name || now.Sub(since) != test.span || bucket != test.bucket {
			t.Fatalf("range %s: name=%s span=%v bucket=%v err=%v", test.name, name, now.Sub(since), bucket, err)
		}
	}
	if _, _, _, err := resolveSignalHistoryRange("invalid", now, 30); err == nil {
		t.Fatal("invalid range accepted")
	}
}

func TestConfirmedSignalHistoryICCIDDoesNotExposeSwitchTarget(t *testing.T) {
	pool := device.NewPool(&config.Config{})
	worker := &device.Worker{ID: "DJI"}
	setNestedPrivateField(t, worker, []string{"state", "Identity", "ICCID"}, "iccid-a")
	setNestedPrivateField(t, worker, []string{"state", "Identity", "Ready"}, true)
	setNestedPrivateField(t, worker, []string{"state", "Identity", "Phase"}, "ready")
	injectWorker(pool, worker)
	s := &Server{pool: pool}
	if got := s.confirmedSignalHistoryICCID("DJI"); got != "iccid-a" {
		t.Fatalf("confirmed ICCID=%q", got)
	}
	setNestedPrivateField(t, worker, []string{"state", "Identity", "TargetICCID"}, "iccid-b")
	setNestedPrivateField(t, worker, []string{"state", "Identity", "Ready"}, false)
	setNestedPrivateField(t, worker, []string{"state", "Identity", "Phase"}, "transitioning")
	if got := s.confirmedSignalHistoryICCID("DJI"); got != "" {
		t.Fatalf("switch target leaked into history query: %q", got)
	}
}
