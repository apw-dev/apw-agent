"""Build a matching portable apw/apwd/APW release without overwriting existing releases."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import time
import zipfile


ROOT = Path(__file__).resolve().parents[1]


def run(args, **kwargs):
    return subprocess.run(args, cwd=ROOT, check=True, capture_output=True, text=True,
                          encoding="utf-8", errors="replace", **kwargs).stdout.strip()


def source_hash():
    names = run(["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"]).split("\0")
    digest = hashlib.sha256()
    for name in sorted(set(names)):
        path = ROOT / name
        if not name or not path.is_file() or path.is_symlink():
            continue
        if path.suffix != ".go" and name not in ("go.mod", "go.sum", "cmd/apw-desktop/go.mod", "cmd/apw-desktop/go.sum"):
            continue
        digest.update(name.replace("\\", "/").encode("utf-8") + b"\0")
        digest.update(path.read_bytes() + b"\0")
    return digest.hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--version", default="0.1.0-rc.1")
    parser.add_argument("--output", type=Path, default=ROOT / ".apw" / "releases")
    parser.add_argument("--install", action="store_true", help="also install the matching pair with Go")
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]*", args.version):
        parser.error("invalid version")
    fingerprint = source_hash()
    revision = run(["git", "rev-parse", "HEAD"])
    built_at = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    goos, goarch = run(["go", "env", "GOOS", "GOARCH"]).splitlines()
    package = args.output.resolve() / f"apw-{args.version}-{goos}-{goarch}-{fingerprint[:12]}"
    package.mkdir(parents=True, exist_ok=False)
    ldflags = " ".join(f"-X github.com/apiwat/apw-agent/internal/buildinfo.{key}={value}"
                       for key, value in {"Version": args.version, "Revision": revision,
                                          "BuiltAt": built_at, "SourceHash": fingerprint}.items())
    suffix = ".exe" if goos == "windows" else ""
    for command in ("apw", "apwd"):
        run(["go", "build", "-mod=readonly", "-trimpath", "-ldflags", ldflags,
             "-o", str(package / (command + suffix)), "./cmd/" + command])
        if source_hash() != fingerprint:
            raise RuntimeError("source changed during build; release was not packaged")
    if goos == "windows":
        run(["go", "-C", "cmd/apw-desktop", "build", "-mod=readonly", "-trimpath",
             "-tags", "desktop,production", "-ldflags", "-H windowsgui " + ldflags,
             "-o", str(package / "apw-desktop.exe"), "."])
        if source_hash() != fingerprint:
            raise RuntimeError("source changed during build; release was not packaged")
    infos = []
    for command, flags in (("apw", ["version", "--json"]), ("apwd", ["--version"])):
        info = json.loads(run([str(package / (command + suffix)), *flags]))
        if info["source_hash"] != fingerprint or info["version"] != args.version:
            raise RuntimeError("binary build metadata mismatch")
        infos.append(info)
    (package / "apw.example.json").write_bytes((ROOT / "apw.example.json").read_bytes())
    (package / "OPERATIONS.md").write_bytes((ROOT / "docs/task-lifecycle.md").read_bytes())
    (package / "RELEASE.md").write_bytes((ROOT / "docs/release.md").read_bytes())
    (package / "ACCEPTANCE.md").write_bytes((ROOT / "docs/acceptance.md").read_bytes())
    manifest = {"build": infos[0], "working_tree_dirty": bool(run(["git","status","--porcelain"])), "files": {}}
    for file in sorted(package.iterdir()):
        manifest["files"][file.name] = hashlib.sha256(file.read_bytes()).hexdigest()
    (package / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    archive = Path(str(package) + ".zip")
    with zipfile.ZipFile(archive, "x", compression=zipfile.ZIP_DEFLATED) as zipped:
        for file in sorted(package.iterdir()):
            zipped.write(file, file.name)
    checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
    Path(str(archive) + ".sha256").write_text(checksum + "  " + archive.name + "\n", encoding="utf-8")
    if args.install:
        run(["go", "install", "-mod=readonly", "-trimpath", "-ldflags", ldflags, "./cmd/apw", "./cmd/apwd"])
    print(json.dumps({"directory": str(package), "archive": str(archive), "sha256": checksum,
                      "source_hash": fingerprint, "installed": args.install}))


if __name__ == "__main__":
    main()
