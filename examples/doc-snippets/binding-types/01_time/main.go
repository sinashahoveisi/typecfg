// Snippet from docs/binding-types.md "time.Time".
package main

import "time"

type Config struct {
	Start time.Time `cfg:"start"`                  // "2026-01-01T00:00:00Z"
	Day   time.Time `cfg:"day" layout:"2006-01-02"` // "2026-07-16"
}

func main() {
	_ = Config{}
}
