//go:build windows

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const restrictedDescription = "restricted (low-integrity token, job UI limits; writes only the worktree)"

type platformState struct {
	token    windows.Token
	pnpmOnce sync.Once
	pnpmErr  error
}

const (
	lowLabel    = "S:(ML;OICI;NW;;;LW)"
	mediumDir   = "S:(ML;OICI;NW;;;ME)"
	mediumFile  = "S:(ML;;NW;;;ME)"
	labelMarker = "(ML;OICI;NW;;;LW)"
)

// openPlatform labels the worktree low integrity (inherited by every file, including ones created
// later by Git or tools), pins .git and .apw back to medium so sandboxed code cannot redirect the
// worktree's repository link or rewrite the snapshot manifest, and creates the low-integrity token.
func openPlatform(root, cache string) (*platformState, error) {
	labeled, err := hasLowLabel(root)
	if err != nil {
		return nil, err
	}
	if !labeled {
		if err := setLabel(root, lowLabel); err != nil {
			return nil, fmt.Errorf("sandbox: label worktree low integrity: %w", err)
		}
	}
	// An explicit medium label precedes the inherited low one, so sandboxed code cannot redirect
	// the worktree's repository link or rewrite the snapshot manifest.
	for _, name := range []string{".git", ".apw"} {
		path := filepath.Join(root, name)
		fi, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		sddl := mediumFile
		if fi.IsDir() {
			sddl = mediumDir
		}
		if err := setLabel(path, sddl); err != nil {
			return nil, fmt.Errorf("sandbox: keep %s at medium integrity: %w", name, err)
		}
	}
	token, err := lowIntegrityToken()
	if err != nil {
		return nil, fmt.Errorf("sandbox: create low-integrity token: %w", err)
	}
	return &platformState{token: token}, nil
}

func (p *platformState) close() { p.token.Close() }

func hasLowLabel(path string) (bool, error) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.LABEL_SECURITY_INFORMATION)
	if err != nil {
		return false, fmt.Errorf("sandbox: read integrity label of %s: %w", path, err)
	}
	return strings.Contains(sd.String(), labelMarker), nil
}

func setLabel(path, sddl string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	sacl, _, err := sd.SACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.LABEL_SECURITY_INFORMATION, nil, nil, nil, sacl)
}

// lowIntegrityToken duplicates the process token as a primary token at low integrity. Processes
// started with it keep the user's identity for reading toolchains but cannot write to objects
// labeled medium (the default for the user's files) or open higher-integrity processes.
func lowIntegrityToken() (windows.Token, error) {
	var current windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ASSIGN_PRIMARY|windows.TOKEN_ADJUST_DEFAULT, &current); err != nil {
		return 0, err
	}
	defer current.Close()
	var dup windows.Token
	if err := windows.DuplicateTokenEx(current, windows.MAXIMUM_ALLOWED, nil, windows.SecurityImpersonation, windows.TokenPrimary, &dup); err != nil {
		return 0, err
	}
	sid, err := windows.CreateWellKnownSid(windows.WinLowLabelSid)
	if err != nil {
		dup.Close()
		return 0, err
	}
	label := windows.Tokenmandatorylabel{Label: windows.SIDAndAttributes{Sid: sid, Attributes: windows.SE_GROUP_INTEGRITY}}
	size := uint32(unsafe.Sizeof(label)) + windows.GetLengthSid(sid)
	if err := windows.SetTokenInformation(dup, windows.TokenIntegrityLevel, (*byte)(unsafe.Pointer(&label)), size); err != nil {
		dup.Close()
		return 0, err
	}
	return dup, nil
}

func (s *Session) restrictedCommand(ctx context.Context, spec Spec) (*Prepared, error) {
	if s.platform == nil {
		return nil, errors.New("sandbox: session is closed")
	}
	cmd := exec.CommandContext(ctx, spec.Args[0], spec.Args[1:]...)
	cmd.Dir = s.root
	cmd.Env = mergeEnv(ScrubEnv(os.Environ(), s.policy.PassEnv), cacheEnv(s.cache, filepath.Join), toolEnv(spec.Args[0], s.cache, filepath.Join),
		[]string{"LOCALAPPDATA=" + filepath.Join(s.cache, "local")}, spec.Env, passed(s.policy.PassEnv))
	if isPnpm(spec.Args[0]) {
		s.platform.pnpmOnce.Do(func() { s.platform.pnpmErr = allowPnpmLocks() })
		if s.platform.pnpmErr != nil {
			return nil, s.platform.pnpmErr
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Token: syscall.Token(s.platform.token)}
	return &Prepared{Cmd: cmd, Limits: s.limits(), Cleanup: func() {}}, nil
}

// allowPnpmLocks lets low-integrity pnpm take its store operation locks. pnpm 11+ keeps them in
// %LOCALAPPDATA%\pnpm-store-operation-locks resolved through the known-folder API (environment
// overrides do not apply), so that one directory, which holds only lock files, is labeled low.
func allowPnpmLocks() error {
	local, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return fmt.Errorf("sandbox: locate LocalAppData for pnpm locks: %w", err)
	}
	dir := filepath.Join(local, "pnpm-store-operation-locks")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("sandbox: create pnpm lock directory: %w", err)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() || fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return fmt.Errorf("sandbox: %s must be a plain directory", dir)
	}
	if labeled, err := hasLowLabel(dir); err != nil || labeled {
		return err
	}
	if err := setLabel(dir, lowLabel); err != nil {
		return fmt.Errorf("sandbox: allow pnpm locks in %s: %w", dir, err)
	}
	return nil
}
