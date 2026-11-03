package platform

import "time"

// Clock is the application time source.
type Clock interface {
	Now() time.Time
}

// SystemClock reads the current system time.
type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now()
}
