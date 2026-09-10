//go:build unix

//nolint:all
package main

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// TestMainRunsAndShutsDownOnInterrupt drives the relay entry point end to end:
// it boots the node, then delivers the interrupt main() waits for.
//
// The test registers its own signal handler first. That installs Go's handler
// process-wide and disables the default terminate-on-SIGINT behaviour, so the
// signal below can never kill the test binary — even if main() returned early
// (a busy port, say) before registering its own.
func TestMainRunsAndShutsDownOnInterrupt(t *testing.T) {
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, os.Interrupt, syscall.SIGINT)
	t.Cleanup(func() { signal.Stop(guard) })

	done := make(chan struct{})
	go func() {
		defer close(done)
		main()
	}()

	// give main() time to reach its own signal.Notify
	time.Sleep(2 * time.Second)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("deliver interrupt: %v", err)
	}

	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("the relay did not shut down after the interrupt")
	}
}
