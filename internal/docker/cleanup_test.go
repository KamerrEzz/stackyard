package docker

import (
	"errors"
	"testing"
)

func TestWrapContainerRemoveErr(t *testing.T) {
	err := wrapContainerRemoveErr(sentinel)
	if err == nil || !errors.Is(err, sentinel) {
		t.Errorf("wrapContainerRemoveErr(%v) = %v, want it to unwrap to sentinel", sentinel, err)
	}
}

func TestWrapVolumeRemoveErr(t *testing.T) {
	err := wrapVolumeRemoveErr(sentinel)
	if err == nil || !errors.Is(err, sentinel) {
		t.Errorf("wrapVolumeRemoveErr(%v) = %v, want it to unwrap to sentinel", sentinel, err)
	}
}

func TestWrapNetworkRemoveErr(t *testing.T) {
	err := wrapNetworkRemoveErr(sentinel)
	if err == nil || !errors.Is(err, sentinel) {
		t.Errorf("wrapNetworkRemoveErr(%v) = %v, want it to unwrap to sentinel", sentinel, err)
	}
}
