//go:build !windows

package main

import "errors"

func editUserPath(string, bool) (bool, error) {
	return false, errors.New("apw path edits the Windows user PATH; on this system add the folder in your shell profile")
}
