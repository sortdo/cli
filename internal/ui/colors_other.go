//go:build !windows

package ui

import "os"

func enableColors(*os.File) bool { return true }
