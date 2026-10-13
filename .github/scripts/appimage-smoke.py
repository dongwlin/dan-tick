#!/usr/bin/env python3
"""Require the system WebKit helper and real frontend content, not just an idle process."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import time

import pyatspi


def webview_processes():
    processes = []
    for entry in Path("/proc").iterdir():
        if not entry.name.isdigit():
            continue
        try:
            executable = os.readlink(entry / "exe")
            status = (entry / "status").read_text()
        except (OSError, PermissionError):
            continue
        if executable.endswith(("/dan-tick", "/WebKitWebProcess")):
            security = {
                line.split(":", 1)[0]: int(line.split(":", 1)[1].strip())
                for line in status.splitlines()
                if line.startswith(("Seccomp:", "NoNewPrivs:"))
            }
            processes.append({"pid": int(entry.name), "executable": executable, **security})
    return processes


def frontend_names(processes):
    app_pids = {process["pid"] for process in processes if process["executable"].endswith("/dan-tick")}
    names = set()
    desktop = pyatspi.Registry.getDesktop(0)
    for app in desktop:
        try:
            if app.get_process_id() not in app_pids:
                continue
            pending = [(app, 0)]
            visited = 0
            while pending and visited < 4000:
                node, depth = pending.pop()
                visited += 1
                if node.name:
                    names.add(node.name)
                if depth < 40:
                    pending.extend((child, depth + 1) for child in node)
        except Exception:
            # AT-SPI can expose a partially constructed tree during page load.
            continue
    return sorted(names)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("launcher_pid", type=int)
    parser.add_argument("results", type=Path)
    parser.add_argument("--timeout", type=float, default=45)
    args = parser.parse_args()
    deadline = time.monotonic() + args.timeout
    required = {"弹幕任务", "执行记录", "直播间"}
    last_error = ""
    while time.monotonic() < deadline:
        try:
            os.kill(args.launcher_pid, 0)
        except ProcessLookupError:
            raise SystemExit("The application exited before its WebView became ready; see application.log.") from None
        processes = webview_processes()
        system_helper = any(
            process["executable"] in {
                "/usr/lib/x86_64-linux-gnu/webkitgtk-6.0/WebKitWebProcess",
                "/usr/libexec/webkitgtk-6.0/WebKitWebProcess",
            }
            for process in processes
        )
        # The outer test container has no seccomp profile. The WebKit helper's
        # own seccomp filter and no-new-privileges flag prove its sandbox ran.
        sandbox_active = any(
            process["executable"].endswith("/WebKitWebProcess")
            and process.get("Seccomp") == 2 and process.get("NoNewPrivs") == 1
            for process in processes
        )
        names = []
        try:
            names = frontend_names(processes)
        except Exception as error:
            last_error = str(error)
        windows = subprocess.run(["xwininfo", "-root", "-tree"], capture_output=True, text=True, check=True).stdout
        report = {
            "processes": processes,
            "system_webkit_helper": system_helper,
            "webkit_sandbox_active": sandbox_active,
            "frontend_names": names,
            "missing_frontend_names": sorted(required.difference(names)),
            "accessibility_error": last_error,
            "window_found": '"dan-tick"' in windows,
        }
        (args.results / "webview.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        if system_helper and sandbox_active and not report["missing_frontend_names"] and report["window_found"]:
            return
        time.sleep(0.5)
    raise SystemExit("The system WebKit helper and rendered Vue accessibility tree did not become ready; see webview.json.")


if __name__ == "__main__":
    main()
