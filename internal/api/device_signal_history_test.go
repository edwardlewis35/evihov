package api

import (
	"testing"
	"time"
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
