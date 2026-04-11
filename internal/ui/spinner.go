package ui

import (
	"fmt"
	"sync"
	"time"
)

// Spinner shows an animated progress indicator while a long-running operation is in progress.
type Spinner struct {
	message string
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
	start   time.Time
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// NewSpinner creates and starts a spinner with the given message.
func NewSpinner(message string) *Spinner {
	s := &Spinner{
		message: message,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		start:   time.Now(),
	}
	go s.run()
	return s
}

func (s *Spinner) run() {
	defer close(s.done)
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case <-s.stop:
			// Clear the spinner line
			mu.Lock()
			fmt.Printf("\r\033[K")
			mu.Unlock()
			return
		case <-ticker.C:
			elapsed := time.Since(s.start).Truncate(time.Second)
			frame := spinnerFrames[i%len(spinnerFrames)]
			mu.Lock()
			fmt.Printf("\r%s%s%s %s %s(%s)%s", Bold, SystemColor, frame, s.message, Dim, elapsed, Reset)
			mu.Unlock()
			i++
		}
	}
}

// Stop stops the spinner and optionally prints a completion message.
func (s *Spinner) Stop(completionMsg string) time.Duration {
	s.once.Do(func() {
		close(s.stop)
	})
	<-s.done
	elapsed := time.Since(s.start)
	if completionMsg != "" {
		PrintSystem("%s %s(%s)%s", completionMsg, Dim, elapsed.Truncate(time.Millisecond), Reset)
	}
	return elapsed
}
