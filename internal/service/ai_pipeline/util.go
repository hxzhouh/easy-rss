package ai_pipeline

import "time"

// durationFromMinutes extracts a time.Duration from an interface with Minutes() method.
func durationFromMinutes(d interface{ Minutes() float64 }) time.Duration {
	return time.Duration(d.Minutes()) * time.Minute
}
