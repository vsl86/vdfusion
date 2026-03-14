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

func AddListener(l func(string)) {
	mu.Lock()
	defer mu.Unlock()
	listeners = append(listeners, l)
}

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
			const maxCapacity = 1024 * 1024
			scanner := bufio.NewScanner(r)
			buf := make([]byte, maxCapacity)
			scanner.Buffer(buf, maxCapacity)

			for scanner.Scan() {
				line := scanner.Text()

				fmt.Fprintln(stdout, line)

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

func RawLog(msg string) {
	if stdout != nil {
		fmt.Fprintln(stdout, msg)
	} else {
		fmt.Fprintln(os.Stdout, msg)
	}
}
