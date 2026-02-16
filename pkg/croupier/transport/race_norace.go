//go:build !race

package transport

// isRaceEnabled returns true when built with -race flag
func isRaceEnabled() bool {
	return false
}
