package dune

import "time"

type RetryPolicy struct {
	MaxAttempts          int
	InitialBackoff       time.Duration
	MaxBackoff           time.Duration
	Jitter               time.Duration
	RetryableStatusCodes []int
}

var defaultRetryPolicy = RetryPolicy{
	MaxAttempts:          5,
	InitialBackoff:       2 * time.Second,
	MaxBackoff:           60 * time.Second,
	Jitter:               250 * time.Millisecond,
	RetryableStatusCodes: []int{429, 500, 502, 503, 504},
}

func (p RetryPolicy) NextBackoff(attempt int) time.Duration {
	b := p.InitialBackoff
	for i := 1; i < attempt; i++ {
		b *= 2
		if b > p.MaxBackoff {
			b = p.MaxBackoff
			break
		}
	}
	if p.Jitter > 0 {
		b += p.Jitter
	}
	return b
}
