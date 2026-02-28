package syslog

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"sync"
)

var (
	listeners []func(string)
	mu        sync.Mutex
	once      sync.Once
	stdout    *os.File
)

// AddListener registers a callback that will be invoked for every line printed to stdout/stderr.
func AddListener(l func(string)) {
	mu.Lock()
	defer mu.Unlock()
	listeners = append(listeners, l)
}

// Start redirects os.Stdout and os.Stderr to an internal pipe and broadcasts output.
func Start() {
	once.Do(func() {
		stdout = os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			return
		}

		os.Stdout = w
		os.Stderr = w
		log.SetOutput(w)

		go func() {
			const maxCapacity = 1024 * 1024 // 1MB buffer for long FFmpeg lines
			scanner := bufio.NewScanner(r)
			buf := make([]byte, maxCapacity)
			scanner.Buffer(buf, maxCapacity)

			for scanner.Scan() {
				line := scanner.Text()

				// 1. Print to original stdout (terminal)
				fmt.Fprintln(stdout, line)

				// 2. Broadcast to listeners (UI)
				mu.Lock()
				if len(listeners) == 0 {
					fmt.Fprintln(stdout, "syslog: WARNING - no listeners registered")
				}
				for _, l := range listeners {
					go l(line)
				}
				mu.Unlock()
			}
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(stdout, "syslog: scanner error: %v\n", err)
			}
		}()
	})
}

// Global logger to use when we need to ensure it's not captured (if needed)
func RawLog(msg string) {
	if stdout != nil {
		fmt.Fprintln(stdout, msg)
	} else {
		fmt.Fprintln(os.Stdout, msg)
	}
}
