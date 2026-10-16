//go:build windows

package tui

import (
	"os"

	"golang.org/x/sys/windows"
)

type rawState struct {
	inHandle  windows.Handle
	outHandle windows.Handle
	origIn    uint32
	origOut   uint32
}

func enableRawTerminal() (*rawState, error) {
	inHandle := windows.Handle(os.Stdin.Fd())
	outHandle := windows.Handle(os.Stdout.Fd())

	var origIn, origOut uint32
	if err := windows.GetConsoleMode(inHandle, &origIn); err != nil {
		return nil, err
	}
	if err := windows.GetConsoleMode(outHandle, &origOut); err != nil {
		return nil, err
	}

	rawIn := origIn &^ (windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_PROCESSED_INPUT)
	rawIn |= windows.ENABLE_VIRTUAL_TERMINAL_INPUT

	rawOut := origOut | windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING

	if err := windows.SetConsoleMode(inHandle, rawIn); err != nil {
		return nil, err
	}
	if err := windows.SetConsoleMode(outHandle, rawOut); err != nil {
		_ = windows.SetConsoleMode(inHandle, origIn)
		return nil, err
	}

	return &rawState{
		inHandle:  inHandle,
		outHandle: outHandle,
		origIn:    origIn,
		origOut:   origOut,
	}, nil
}

func (s *rawState) restore() {
	if s == nil {
		return
	}
	_ = windows.SetConsoleMode(s.inHandle, s.origIn)
	_ = windows.SetConsoleMode(s.outHandle, s.origOut)
}

func terminalWidth() int {
	outHandle := windows.Handle(os.Stdout.Fd())
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(outHandle, &info); err == nil {
		w := int(info.Window.Right - info.Window.Left + 1)
		if w >= 60 {
			return w
		}
	}
	return 88
}
