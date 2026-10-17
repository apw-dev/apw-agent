package tui

import (
	"strings"
	"testing"
)

func TestInputPreservesThaiAcrossChunksAndPastedCommands(t *testing.T) {
	var decoder keyDecoder
	input := "/review ตรวจสอบโค้ดภาษาไทย\r\n/new\n"
	var actual strings.Builder
	for _, b := range []byte(input) {
		decoder.push([]byte{b})
		for key := decoder.pop(); key != nil; key = decoder.pop() {
			actual.Write(key)
		}
	}
	if actual.String() != strings.ReplaceAll(input, "\r\n", "\r") {
		t.Fatalf("input changed: %q", actual.String())
	}
	decoder.push([]byte("first\nsecond\n"))
	var lines []string
	var line strings.Builder
	for key := decoder.pop(); key != nil; key = decoder.pop() {
		if string(key) == "\n" {
			lines = append(lines, line.String())
			line.Reset()
		} else {
			line.Write(key)
		}
	}
	if len(lines) != 2 || lines[0] != "first" || lines[1] != "second" {
		t.Fatalf("pasted commands were dropped: %v", lines)
	}
}

func TestCoreCommandsAreDiscoverable(t *testing.T) {
	for _, name := range []string{"/review", "/preview", "/task", "/tasks", "/diff", "/watch", "/apply", "/cancel", "/trust"} {
		found := false
		for _, command := range DefaultCommands() {
			if command.Name == name {
				found = true
			}
		}
		if !found {
			t.Errorf("command missing: %s", name)
		}
	}
}
