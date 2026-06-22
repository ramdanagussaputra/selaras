package security

import "time"

// SystemClock implements domain/auth.Clock with the wall clock.
type SystemClock struct{}

// Now returns the current time.
func (SystemClock) Now() time.Time { return time.Now() }
