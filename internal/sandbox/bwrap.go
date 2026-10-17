package sandbox

// bwrapArgs builds a bubblewrap command line: everything read-only except the worktree, a private
// /tmp, new namespaces (no network unless allowed) and the sandbox dying with its parent.
func bwrapArgs(root string, network bool, command []string) []string {
	args := []string{"--die-with-parent", "--new-session", "--unshare-all"}
	if network {
		args = append(args, "--share-net")
	}
	args = append(args, "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "--tmpfs", "/tmp",
		"--bind", root, root, "--chdir", root, "--")
	return append(args, command...)
}
