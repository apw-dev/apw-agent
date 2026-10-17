//go:build linux

package sandbox

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

const restrictedDescription = "restricted (bubblewrap: read-only root, writable worktree, private /tmp)"

type platformState struct{ bwrap string }

func openPlatform(string, string) (*platformState, error) {
	path, err := exec.LookPath("bwrap")
	if err != nil {
		return nil, errors.New("sandbox: restricted mode on Linux requires bubblewrap (bwrap); install it or use sandbox.mode docker")
	}
	return &platformState{bwrap: path}, nil
}

func (p *platformState) close() {}

func (s *Session) restrictedCommand(ctx context.Context, spec Spec) (*Prepared, error) {
	if s.platform == nil {
		return nil, errors.New("sandbox: session is closed")
	}
	cmd := exec.CommandContext(ctx, s.platform.bwrap, bwrapArgs(s.root, s.network(spec), spec.Args)...)
	cmd.Dir = s.root
	cmd.Env = mergeEnv(ScrubEnv(os.Environ(), s.policy.PassEnv), cacheEnv(s.cache, filepath.Join), toolEnv(spec.Args[0], s.cache, filepath.Join), spec.Env, passed(s.policy.PassEnv))
	return &Prepared{Cmd: cmd, Limits: s.limits(), Cleanup: func() {}}, nil
}
