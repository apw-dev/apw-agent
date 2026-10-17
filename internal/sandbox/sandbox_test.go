package sandbox

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/apiwat/apw-agent/internal/config"
)

func TestFromConfigDefaults(t *testing.T) {
	p := FromConfig(config.SandboxConfig{})
	if p.Mode != ModeOff || p.Network != NetworkInstall || p.Active() {
		t.Fatalf("defaults = %+v", p)
	}
	if !FromConfig(config.SandboxConfig{Mode: "docker", Image: "node:22"}).Active() {
		t.Fatal("docker policy not active")
	}
}

func TestScrubEnvRemovesCredentials(t *testing.T) {
	in := []string{"PATH=/bin", "ANTHROPIC_API_KEY=a", "GITHUB_TOKEN=b", "DB_PASSWORD=c", "APW_TELEGRAM_ALLOW=1", "NPM_TOKEN=d", "HOME=/h", "aws_secret_access_key=e", "broken"}
	out := strings.Join(ScrubEnv(in, []string{"NPM_TOKEN"}), "\n")
	for _, gone := range []string{"ANTHROPIC_API_KEY", "GITHUB_TOKEN", "DB_PASSWORD", "APW_TELEGRAM_ALLOW", "aws_secret_access_key", "broken"} {
		if strings.Contains(out, gone) {
			t.Fatalf("%s kept:\n%s", gone, out)
		}
	}
	for _, kept := range []string{"PATH=/bin", "HOME=/h", "NPM_TOKEN=d"} {
		if !strings.Contains(out, kept) {
			t.Fatalf("%s removed:\n%s", kept, out)
		}
	}
}

func TestMergeEnvOverrides(t *testing.T) {
	got := mergeEnv([]string{"A=1", "B=2"}, []string{"B=3"}, []string{"C=4"})
	if strings.Join(got, ",") != "A=1,B=3,C=4" {
		t.Fatalf("merge = %v", got)
	}
}

func TestBwrapArgs(t *testing.T) {
	args := strings.Join(bwrapArgs("/wt", false, []string{"npm", "test"}), " ")
	for _, want := range []string{"--unshare-all", "--ro-bind / /", "--bind /wt /wt", "--chdir /wt", "--tmpfs /tmp", "-- npm test"} {
		if !strings.Contains(args, want) {
			t.Fatalf("bwrap args missing %q: %s", want, args)
		}
	}
	if strings.Contains(args, "--share-net") {
		t.Fatal("network shared for a non-install stage")
	}
	if !strings.Contains(strings.Join(bwrapArgs("/wt", true, []string{"npm", "ci"}), " "), "--share-net") {
		t.Fatal("install stage has no network")
	}
}

func TestDockerCommand(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not installed")
	}
	root := t.TempDir()
	t.Setenv("OPENAI_API_KEY", "must-not-pass")
	s, err := Open(Policy{Mode: ModeDocker, Image: "node:22", Network: NetworkInstall, MemoryMB: 512}, root)
	if err != nil {
		t.Fatal(err)
	}
	test, err := s.Command(context.Background(), Spec{Args: []string{"npm", "test"}, Env: []string{"CI=true"}})
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(test.Cmd.Args, " ")
	for _, want := range []string{"run --rm --init", "--network none", "--cap-drop ALL", "--memory 512m", root + ":/workspace", "-w /workspace", "node:22 npm test", "GOCACHE=/workspace/.apw-sandbox/go-build"} {
		if !strings.Contains(args, want) {
			t.Fatalf("docker args missing %q: %s", want, args)
		}
	}
	if strings.Contains(args, "must-not-pass") {
		t.Fatal("host credential passed into the container")
	}
	install, err := s.Command(context.Background(), Spec{Args: []string{"npm", "ci"}, Install: true, Optional: true})
	if err != nil {
		t.Fatal(err)
	}
	if args := strings.Join(install.Cmd.Args, " "); !strings.Contains(args, "--network bridge") || !strings.Contains(args, "command -v") {
		t.Fatalf("install args = %s", args)
	}
}
