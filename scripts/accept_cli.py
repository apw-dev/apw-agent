"""Actual CLI/daemon crash recovery and console cancellation; local model fixture only."""
import argparse
import ctypes
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def console_helper(exe, cfg, directory):
    directory = Path(directory)
    callback = None
    if os.name == "nt":
        # This helper owns a separate hidden console; events cannot reach the user's console.
        handler = ctypes.WINFUNCTYPE(ctypes.c_bool, ctypes.c_uint)
        callback = handler(lambda event: True)
        if not ctypes.windll.kernel32.SetConsoleCtrlHandler(callback, True):
            raise RuntimeError("cannot register isolated console handler")
    with (directory / "ctrl.stdout").open("w", encoding="utf-8") as out, (directory / "ctrl.stderr").open("w", encoding="utf-8") as err:
        child = subprocess.Popen([exe, "review", "--config", cfg, "--request-id", "ctrl-c", "slow review"], stdout=out, stderr=err)
        try:
            wait_for(lambda: (directory / "review.started").exists())
            if os.name == "nt":
                if not ctypes.windll.kernel32.GenerateConsoleCtrlEvent(0, 0):
                    raise RuntimeError("cannot send isolated CTRL_C_EVENT")
            else:
                child.send_signal(signal.SIGINT)
            code = child.wait(timeout=15)
            (directory / "ctrl.result").write_text(str(code))
            if code != 130:
                raise RuntimeError(f"CLI interrupt exit was {code}, expected 130")
        finally:
            if child.poll() is None:
                child.terminate()
                child.wait(timeout=10)


def wait_for(predicate, timeout=40):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        value = predicate()
        if value:
            return value
        time.sleep(.03)
    raise TimeoutError("acceptance condition did not arrive")


def main(binary_dir, output):
    binaries = Path(binary_dir).resolve()
    exe = str(binaries / ("apw.exe" if os.name == "nt" else "apw"))
    root = Path(tempfile.mkdtemp(prefix="apw-recovery-accept-"))
    repo = root / "workspace with spaces"
    repo.mkdir()
    cfg = root / "config.json"
    env = dict(os.environ)
    # Keep the original PATH (which may contain an older apwd); the absolute CLI must prefer its sibling.
    expected_build=json.loads(subprocess.check_output([exe,"version","--json"],env=env,text=True,encoding="utf-8"))["source_hash"]
    env["APW_TELEGRAM_TOKEN"] = env["APW_TELEGRAM_ALLOW"] = ""
    for name, value in {"main.go": 'package main\n\nfunc Greet() string {\n\treturn "hi"\n}\n\nfunc main() {}\n', "helper.go": 'package main\n\nfunc Helper() string {\n\treturn "old"\n}\n', "go.mod": "module example.com/recovery\n\ngo 1.24\n"}.items():
        (repo / name).write_bytes(value.encode())
    for args in [["init", "-q"], ["add", "."], ["-c", "user.name=Acceptance", "-c", "user.email=acceptance@example.test", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + str(root), "commit", "-qm", "fixture"]]:
        subprocess.run(["git", "-C", str(repo), *args], check=True, capture_output=True)
    helper_started = threading.Event()
    release = threading.Event()
    counts = {"main": 0, "helper": 0, "planning": 0, "audit": 0}
    block_helper = [True]

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            prompt = json.loads(self.rfile.read(int(self.headers["Content-Length"])))["messages"][0]["content"]
            if prompt.startswith("Review the supplied source only."):
                (root / "review.started").write_text("started")
                release.wait(60)
                content = "reviewed"
            elif prompt.startswith("You are the ") and "Auditor for a code change" in prompt:
                counts["audit"] += 1
                content = "- fixture audit: no findings"
            elif "Provide surgical patch for helper.go." in prompt:
                counts["helper"] += 1
                if block_helper[0]:
                    helper_started.set()
                    release.wait(60)
                content = json.dumps({"old_content": 'return "old"', "new_content": 'return "new"'})
            elif "old_content" in prompt:
                counts["main"] += 1
                content = json.dumps({"old_content": 'return "hi"', "new_content": 'return "hello"'})
            else:
                counts["planning"] += 1
                content = json.dumps({"objective": "change both functions", "affected_files": ["main.go", "helper.go"], "subtasks": [{"id": "one", "action": "patch", "target_file": "main.go", "instruction": "return hello"}, {"id": "two", "action": "patch", "target_file": "helper.go", "instruction": "return new"}], "acceptance_criteria": ["updated"], "checks": [{"criterion": "updated", "kind": "contains", "file": "helper.go", "value": 'return "new"'}]})
            data = json.dumps({"model": "fixture", "choices": [{"message": {"role": "assistant", "content": content}, "finish_reason": "stop"}]}).encode()
            try:
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(data)
            except (BrokenPipeError, ConnectionResetError, ConnectionAbortedError):
                pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    cfg.write_text(json.dumps({"workspace": str(repo), "providers": {"fixture": {"type": "local", "model": "fixture", "endpoint": f"http://127.0.0.1:{server.server_port}"}}, "roles": {"architect": "fixture", "reflex": "fixture"}, "policy": {"trusted": True}, "budget": {"max_repairs": 1, "task_timeout_sec": 120}}))

    def call(command, *args):
        result = subprocess.run([exe, command, "--config", str(cfg), *args], env=env, capture_output=True, text=True, encoding="utf-8", errors="replace", timeout=45)
        if result.returncode:
            raise RuntimeError(command + ": " + result.stderr)
        return result.stdout

    def jobs():
        state = json.loads((repo / ".apw/daemon.json").read_text())
        request = urllib.request.Request(state["address"] + "/v1/tasks", headers={"Authorization": "Bearer " + state["token"]})
        return json.load(urllib.request.urlopen(request, timeout=5))

    report = {"fixture_directory": str(root), "real_provider_calls": 0}
    runner = None
    try:
        with (root / "initial.stdout").open("w") as out, (root / "initial.stderr").open("w") as err:
            runner = subprocess.Popen([exe, "run", "--config", str(cfg), "--request-id", "recovery", "change both functions"], env=env, stdout=out, stderr=err)
        wait_for(helper_started.is_set)
        initial = next(job for job in jobs() if job["request_id"] == "recovery")
        checkpoint = next(json.loads(p.read_text()) for p in (repo / ".apw/plans").glob("*.checkpoint.json"))
        assert checkpoint["last_success_step"] == 1
        state = json.loads((repo / ".apw/daemon.json").read_text())
        assert Path(state["workspace"]) == repo and Path(state["config_path"]) == cfg
        assert state["build"]["source_hash"] == expected_build
        report["matching_sibling_daemon_with_original_path"] = True
        os.kill(state["pid"], signal.SIGTERM)  # Windows TerminateProcess: deliberate owned-daemon crash.
        runner.wait(timeout=20)
        before_restart = dict(counts)
        block_helper[0] = False
        release.set()
        call("daemon", "start")
        recovered = json.loads(call("task", initial["id"]))
        assert recovered["status"] == "interrupted" and counts == before_restart
        assert 'return "hi"' in (repo / "main.go").read_text()
        call("resume", initial["id"])
        assert counts["planning"] == 1 and counts["main"] == 1 and counts["helper"] == 2
        assert json.loads(call("task", initial["id"]))["status"] == "ready_to_apply"
        call("apply", initial["id"])
        assert 'return "hello"' in (repo / "main.go").read_text() and 'return "new"' in (repo / "helper.go").read_text()
        report["crash_resume_exact_step"] = True
        report["restart_did_not_replay_model"] = True
        release.clear()
        kwargs = {}
        if os.name == "nt":
            startup = subprocess.STARTUPINFO()
            startup.dwFlags |= subprocess.STARTF_USESHOWWINDOW
            startup.wShowWindow = 0
            kwargs = {"creationflags": subprocess.CREATE_NEW_CONSOLE, "startupinfo": startup}
        # Force UTF-8 on both ends: a non-UTF-8 locale (e.g. cp874) otherwise breaks decoding
        # in subprocess's reader thread, leaving stderr None and hiding the helper's real error.
        helper_env = dict(env, PYTHONIOENCODING="utf-8")
        helper = subprocess.run([sys.executable, __file__, "--console-helper", exe, str(cfg), str(root)], env=helper_env, capture_output=True, text=True, encoding="utf-8", errors="replace", timeout=65, **kwargs)
        if helper.returncode:
            detail = (helper.stderr or "").strip() or (helper.stdout or "").strip() or f"exit code {helper.returncode}, no output"
            raise RuntimeError("console helper failed: " + detail)
        cancelled = wait_for(lambda: next((j for j in jobs() if j["request_id"] == "ctrl-c" and j["status"] == "cancelled"), None))
        report["real_console_ctrl_c_exit_130"] = (root / "ctrl.result").read_text() == "130"
        report["cancel_persisted"] = cancelled["cancel_requested"]
        report["passed"] = True
    except Exception as exc:
        report.update(passed=False, error=str(exc))
    finally:
        release.set()
        subprocess.run([exe, "daemon", "--config", str(cfg), "stop"], env=env, capture_output=True, timeout=15)
        if runner is not None and runner.poll() is None:
            runner.terminate()
            runner.wait(timeout=10)
        server.shutdown()
        Path(output).write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(report))
    return 0 if report.get("passed") else 1


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "--console-helper":
        console_helper(*sys.argv[2:])
    else:
        parser = argparse.ArgumentParser()
        parser.add_argument("--bin", required=True)
        parser.add_argument("--output", required=True)
        args = parser.parse_args()
        sys.exit(main(args.bin, args.output))
