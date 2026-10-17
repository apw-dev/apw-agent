//go:build !windows && !linux

package sandbox

import (
	"context"
	"errors"
)

const restrictedDescription = "restricted (unavailable on this platform)"

type platformState struct{}

func openPlatform(string, string) (*platformState, error) {
	return nil, errors.New("sandbox: restricted mode is available on Windows and Linux; use sandbox.mode docker on this platform")
}

func (p *platformState) close() {}

func (s *Session) restrictedCommand(context.Context, Spec) (*Prepared, error) {
	return nil, errors.New("sandbox: restricted mode is not available on this platform")
}
