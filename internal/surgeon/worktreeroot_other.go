//go:build !unix && !windows

package surgeon

func restrictDir(string) error   { return nil }
func checkDirOwner(string) error { return nil }
