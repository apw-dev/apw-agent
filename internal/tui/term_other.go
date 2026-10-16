//go:build !windows

package tui

import "errors"

type rawState struct{}

func enableRawTerminal() (*rawState, error) {
	return nil, errors.New("raw console mode only enabled on interactive Windows console")
}

func (s *rawState) restore() {}

func terminalWidth() int {
	return 88
}
