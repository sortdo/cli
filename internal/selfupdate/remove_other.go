//go:build !windows

package selfupdate

import "os"

// Remove deletes the program.
func Remove(executable string) error { return os.Remove(executable) }
