package clock

import "time"

type Clock interface {
	Now() time.Time
}

type Func func() time.Time

func (f Func) Now() time.Time {
	if f == nil {
		return time.Now()
	}
	return f()
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now()
}
