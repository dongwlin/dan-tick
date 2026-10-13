#!/usr/bin/env python3
"""Check that the packaged GIO TLS module performs real TLS.

The application reaches every HTTPS API through libsoup, which uses the GIO TLS
backend. This script loads the GIO modules from the AppImage, starts a local
HTTPS server that uses a test certificate, and requires the handshake to succeed
with certificate validation against the system trust store. The server only
serves generated data and never talks to a production API.
"""
import os
from pathlib import Path
import socket
import ssl
import sys
import threading

CERTIFICATE_DIRECTORY = Path("/opt/dan-tick-smoke-tls")


def start_server():
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(CERTIFICATE_DIRECTORY / "server.crt", CERTIFICATE_DIRECTORY / "server.key")
    listener = socket.socket()
    listener.bind(("127.0.0.1", 0))
    listener.listen(1)
    port = listener.getsockname()[1]

    def serve():
        try:
            raw, _ = listener.accept()
            with context.wrap_socket(raw, server_side=True) as connection:
                connection.recv(4096)
                connection.sendall(
                    b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok"
                )
        except OSError:
            pass
        finally:
            listener.close()

    thread = threading.Thread(target=serve, daemon=True)
    thread.start()
    return port, thread


def main():
    if len(sys.argv) != 3:
        raise SystemExit("Usage: appimage-tls-check.py <AppDir> <output file>")
    appdir = Path(sys.argv[1]).resolve()
    output = Path(sys.argv[2])
    libdir = appdir / "usr/lib/x86_64-linux-gnu"
    # Load the bundled GIO, its TLS module and their dependencies instead of the
    # copies the test harness was installed with.
    os.environ["LD_LIBRARY_PATH"] = f"{libdir}:{os.environ.get('LD_LIBRARY_PATH', '')}".rstrip(":")
    os.environ["GIO_EXTRA_MODULES"] = str(libdir / "gio/modules")
    # The packaged GLib reads its settings schemas from XDG_DATA_DIRS, so this has
    # to point at the package like AppRun does. Otherwise distributions that keep
    # their schemas elsewhere fail before the TLS handshake is even attempted.
    os.environ["XDG_DATA_DIRS"] = (
        f"{appdir}/usr/share:{os.environ.get('XDG_DATA_DIRS', '/usr/local/share:/usr/share')}"
    )
    os.environ["GSETTINGS_SCHEMA_DIR"] = str(appdir / "usr/share/glib-2.0/schemas")

    import gi
    gi.require_version("Gio", "2.0")
    from gi.repository import Gio, GLib

    lines = []
    backend = Gio.tls_backend_get_default()
    lines.append(f"tls-backend: {type(backend).__name__ if backend is not None else 'missing'}")
    if backend is None:
        output.write_text("\n".join(lines) + "\n")
        raise SystemExit("no GIO TLS backend is available from the packaged modules")

    database = backend.get_default_database()
    lines.append(f"default-database: {type(database).__name__ if database is not None else 'missing'}")

    port, thread = start_server()
    lines.append(f"https-server-port: {port}")
    client = Gio.SocketClient()
    client.set_tls(True)
    address = Gio.NetworkAddress.new("localhost", port)
    try:
        connection = client.connect(address, None)
        stream = connection.get_output_stream()
        stream.write_all(b"GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n", None)
        lines.append("handshake: ok")
        # SocketClient wraps the TLS connection, so the peer certificate belongs
        # to the stream the connection was built from.
        tls_connection = connection.get_base_io_stream() if hasattr(connection, "get_base_io_stream") else connection
        peer = tls_connection.get_peer_certificate() if hasattr(tls_connection, "get_peer_certificate") else None
        lines.append(f"peer-certificate: {peer is not None}")
        connection.close(None)
    except GLib.Error as error:
        lines.append(f"handshake: failed: {error.message}")
        thread.join(timeout=5)
        output.write_text("\n".join(lines) + "\n")
        raise SystemExit(f"TLS handshake with the packaged GIO module failed: {error.message}") from None
    thread.join(timeout=5)

    loaded = []
    for line in Path("/proc/self/maps").read_text().splitlines():
        for token in line.split():
            if token.startswith("/") and ".so" in token:
                loaded.append(token)
    lines.append("tls-module-from-package: " + str(
        any(path.startswith(str(libdir)) and "giognutls" in path for path in loaded)
    ))
    output.write_text("\n".join(lines) + "\n")


if __name__ == "__main__":
    main()
