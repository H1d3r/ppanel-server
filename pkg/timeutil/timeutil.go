// Package timeutil provides centralized timezone handling for the application.
// Call LoadLocation once during initialization to set the canonical timezone,
// then use Now() and Location() throughout business logic instead of time.Now()
// and time.Local.
package timeutil

import (
	"sync"
	"time"
)

var (
	mu   sync.RWMutex
	loc  *time.Location
	name string
)

// LoadLocation loads the timezone by name (e.g., "Asia/Shanghai", "UTC").
// Must be called once at startup before any other function in this package.
// An unknown name is an error and leaves the current timezone in place.
func LoadLocation(tzName string) error {
	mu.Lock()
	defer mu.Unlock()

	l, err := time.LoadLocation(tzName)
	if err != nil {
		return err
	}
	loc = l
	name = tzName
	return nil
}

// Location returns the canonical application timezone.
// Falls back to time.Local if LoadLocation was never called.
func Location() *time.Location {
	mu.RLock()
	defer mu.RUnlock()
	if loc == nil {
		return time.Local
	}
	return loc
}

// LocationName returns the configured timezone name, or "Local" when
// LoadLocation was never called.
func LocationName() string {
	mu.RLock()
	defer mu.RUnlock()
	if name == "" {
		return "Local"
	}
	return name
}

// Now returns the current time in the application timezone.
// Falls back to time.Now() if LoadLocation was never called.
func Now() time.Time {
	return time.Now().In(Location())
}
