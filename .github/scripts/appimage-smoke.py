#!/usr/bin/env python3
"""Prove the packaged application runs on its own GTK 4 / WebKitGTK 6.0 runtime.

Everything checked here is read from the running system: which libraries the UI
process and the WebKit auxiliary processes mapped, whether the helpers came from
the AppImage, whether the WebKit sandbox is active, and which text the rendered
frontend exposes through AT-SPI. A data driven label is required as well, because
it only appears after the frontend received an answer from the Go bindings.
"""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import time

import pyatspi

# Libraries that must come from inside the AppImage. The list covers the GUI and
# JavaScript stack the package is responsible for; glibc, libstdc++ and the GPU
# drivers are allowed to come from the host.
BUNDLED_LIBRARIES = (
    "libgtk-4.so",
    "libwebkitgtk-6.0.so",
    "libjavascriptcoregtk-6.0.so",
    "libsoup-3.0.so",
    "libgio-2.0.so",
    "libglib-2.0.so",
    "libgdk_pixbuf-2.0.so",
    "libpango-1.0.so",
    "libcairo.so",
    "libharfbuzz.so",
    "libepoxy.so",
)

# Static navigation labels plus one label that is only rendered after the
# frontend received an empty task list from the Go service.
REQUIRED_NAMES = {"弹幕任务", "执行记录", "直播间", "暂无即将执行的任务"}


def process_information():
    processes = []
    for entry in Path("/proc").iterdir():
        if not entry.name.isdigit():
            continue
        try:
            executable = os.readlink(entry / "exe")
            status = (entry / "status").read_text()
            namespaces = {
                name: os.readlink(entry / "ns" / name)
                for name in ("user", "mnt", "pid")
            }
        except (OSError, PermissionError):
            continue
        if not executable.endswith(("/dan-tick", "/WebKitWebProcess", "/WebKitNetworkProcess", "/WebKitGPUProcess")):
            continue
        security = {
            line.split(":", 1)[0]: int(line.split(":", 1)[1].strip())
            for line in status.splitlines()
            if line.startswith(("Seccomp:", "NoNewPrivs:"))
        }
        processes.append({
            "pid": int(entry.name),
            "executable": executable,
            "namespaces": namespaces,
            **security,
        })
    return processes


def loaded_libraries(pid):
    libraries = []
    try:
        maps = Path(f"/proc/{pid}/maps").read_text()
    except OSError:
        return libraries
    for line in maps.splitlines():
        match = re.search(r"\s(\S+\.so[^\s]*)$", line)
        if match:
            libraries.append(match.group(1))
    return sorted(set(libraries))


def library_report(processes, bundle_prefix):
    report = {}
    for process in processes:
        libraries = loaded_libraries(process["pid"])
        bundled = [library for library in libraries if library.startswith(bundle_prefix)]
        outside = [
            library
            for library in libraries
            if not library.startswith(bundle_prefix) and library.startswith("/")
        ]
        from_host = [
            library
            for library in libraries
            if any(Path(library).name.startswith(name) for name in BUNDLED_LIBRARIES)
            and not library.startswith(bundle_prefix)
        ]
        report[process["executable"]] = {
            "bundled": bundled,
            "outside_bundle": outside,
            "gui_stack_outside_bundle": from_host,
        }
    return report


def frontend_names(app_pids):
    names = set()
    desktop = pyatspi.Registry.getDesktop(0)
    for app in desktop:
        try:
            if app.get_process_id() not in app_pids:
                continue
            pending = [(app, 0)]
            visited = 0
            while pending and visited < 6000:
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
    parser.add_argument("--bundle-prefix", default=None,
                        help="directory the AppImage runs from; derived from the running application when omitted")
    parser.add_argument("--timeout", type=float, default=45)
    args = parser.parse_args()

    deadline = time.monotonic() + args.timeout
    last_report = {}
    while time.monotonic() < deadline:
        try:
            os.kill(args.launcher_pid, 0)
        except ProcessLookupError:
            raise SystemExit("The application exited before its WebView became ready; see application.log.") from None

        processes = process_information()
        app_pids = {process["pid"] for process in processes if process["executable"].endswith("/dan-tick")}
        helpers = [process for process in processes if "/WebKit" in process["executable"]]

        # While the AppImage is mounted by the runtime, or extracted for the
        # extract-and-run modes, the application runs from its own bundle
        # directory: <bundle>/usr/bin/dan-tick.
        bundle_prefix = args.bundle_prefix
        if not bundle_prefix:
            for process in processes:
                if process["executable"].endswith("/usr/bin/dan-tick"):
                    bundle_prefix = str(Path(process["executable"]).parents[2])
                    break
        if not bundle_prefix:
            time.sleep(0.5)
            continue

        packaged_helpers = [process for process in helpers if process["executable"].startswith(bundle_prefix)]
        web_processes = [process for process in packaged_helpers if process["executable"].endswith("/WebKitWebProcess")]

        # The helper's own seccomp filter, its no-new-privileges flag and its own
        # user namespace prove that the WebKit sandbox ran with bubblewrap.
        sandboxed = []
        for process in web_processes:
            try:
                init_user_ns = os.readlink("/proc/1/ns/user")
                init_mnt_ns = os.readlink("/proc/1/ns/mnt")
            except OSError:
                init_user_ns = init_mnt_ns = ""
            if (process.get("Seccomp") == 2
                    and process.get("NoNewPrivs") == 1
                    and process["namespaces"].get("user") not in ("", init_user_ns)
                    and process["namespaces"].get("mnt") not in ("", init_mnt_ns)):
                sandboxed.append(process["pid"])

        report = {
            "bundle_prefix": bundle_prefix,
            "processes": processes,
            "libraries": library_report(processes, bundle_prefix),
            "packaged_helpers": [process["executable"] for process in packaged_helpers],
            "sandboxed_web_processes": sandboxed,
        }

        names = []
        try:
            names = frontend_names(app_pids)
        except Exception as error:  # pragma: no cover - diagnostics only
            report["accessibility_error"] = str(error)
        report["frontend_names"] = names
        report["missing_frontend_names"] = sorted(REQUIRED_NAMES.difference(names))

        windows = subprocess.run(["xwininfo", "-root", "-tree"], capture_output=True, text=True, check=True).stdout
        report["window_found"] = '"dan-tick"' in windows

        injected_bundle = any(
            any(Path(library).name.startswith("libwebkitgtkinjectedbundle") for library in entry["bundled"])
            for name, entry in report["libraries"].items()
            if name.endswith("/WebKitWebProcess")
        )
        report["injected_bundle_from_bundle"] = injected_bundle

        gui_stack_outside = {
            name: entry["gui_stack_outside_bundle"]
            for name, entry in report["libraries"].items()
            if entry["gui_stack_outside_bundle"]
        }
        report["gui_stack_outside_bundle"] = gui_stack_outside

        (args.results / "webview.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        last_report = report

        if (packaged_helpers
                and sandboxed
                and injected_bundle
                and not gui_stack_outside
                and not report["missing_frontend_names"]
                and report["window_found"]):
            return
        time.sleep(0.5)

    raise SystemExit(
        "The packaged WebKit runtime, its sandbox and the rendered Vue page did not become ready: "
        + json.dumps({key: last_report.get(key) for key in (
            "packaged_helpers", "sandboxed_web_processes", "injected_bundle_from_bundle",
            "gui_stack_outside_bundle", "missing_frontend_names", "window_found")}, ensure_ascii=False)
    )


if __name__ == "__main__":
    main()
