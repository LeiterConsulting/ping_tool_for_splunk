//go:build !windows

package main

import (
	"fmt"
	"os"
	"strings"
)

func runServiceCommand(args []string) (bool, int) {
	if len(args) == 0 || (!strings.EqualFold(args[0], "service") && !strings.EqualFold(args[0], "service-run")) {
		return false, 0
	}
	fmt.Fprintln(os.Stderr, "native service management is available only in the Windows build")
	return true, 2
}
