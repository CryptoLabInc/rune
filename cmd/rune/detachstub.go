//go:build !unix

package main

import "os/exec"

func setDetached(*exec.Cmd) bool { return false } // no-op on non-unix platforms
