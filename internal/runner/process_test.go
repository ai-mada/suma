package runner

import (
	"errors"
	"testing"
	"time"
)

func TestWaitForNativeCompletion(t *testing.T) {
	done := make(chan error, 1)
	want := errors.New("native wait result")
	done <- want
	if result, waited := waitForNativeCompletion(done, time.Second); !waited || result != want {
		t.Fatal("original wait result was lost", result, waited)
	}
	finished := make(chan bool, 1)
	go func() {
		_, waited := waitForNativeCompletion(done, 10*time.Millisecond)
		finished <- waited
	}()
	select {
	case waited := <-finished:
		if waited {
			t.Fatal("timeout reported that the native wait returned")
		}
	case <-time.After(time.Second):
		t.Fatal("native wait did not return at its bound")
	}
}
