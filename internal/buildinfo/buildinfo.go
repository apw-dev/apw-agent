package buildinfo

import "runtime"

// Package builders set these for both executables with the same source fingerprint.
var Version = "0.1.0-alpha"
var Revision = "unrecorded"
var BuiltAt = "unrecorded"
var SourceHash = "unrecorded"

type Info struct {
	Version    string `json:"version"`
	Revision   string `json:"revision"`
	BuiltAt    string `json:"built_at"`
	SourceHash string `json:"source_hash"`
	GoVersion  string `json:"go_version"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
}

func Current() Info {
	return Info{Version: Version, Revision: Revision, BuiltAt: BuiltAt, SourceHash: SourceHash, GoVersion: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH}
}
