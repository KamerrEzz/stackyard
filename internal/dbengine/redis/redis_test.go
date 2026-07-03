package redis

import (
	"errors"
	"testing"
	"time"
)

func TestTranslateTTL_NegativeTwoReturnsKeyNotFound(t *testing.T) {
	got, err := translateTTL(-2 * time.Nanosecond)
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("translateTTL(-2ns) error = %v, want ErrKeyNotFound", err)
	}
	if got != 0 {
		t.Errorf("translateTTL(-2ns) duration = %v, want 0", got)
	}
}

func TestTranslateTTL_NegativeOneReturnsNoExpirySentinel(t *testing.T) {
	got, err := translateTTL(-1 * time.Nanosecond)
	if err != nil {
		t.Fatalf("translateTTL(-1ns) failed: %v", err)
	}
	if got != -1 {
		t.Errorf("translateTTL(-1ns) duration = %v, want -1", got)
	}
}

func TestTranslateTTL_PositiveDurationPassesThroughUnchanged(t *testing.T) {
	want := 42 * time.Second
	got, err := translateTTL(want)
	if err != nil {
		t.Fatalf("translateTTL(%v) failed: %v", want, err)
	}
	if got != want {
		t.Errorf("translateTTL(%v) = %v, want it unchanged", want, got)
	}
}

func TestTranslateTTL_ZeroDurationPassesThroughUnchanged(t *testing.T) {
	got, err := translateTTL(0)
	if err != nil {
		t.Fatalf("translateTTL(0) failed: %v", err)
	}
	if got != 0 {
		t.Errorf("translateTTL(0) = %v, want 0", got)
	}
}
