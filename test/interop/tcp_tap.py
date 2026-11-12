#!/usr/bin/env python3
"""TCP forwarder that logs the first line of every connection ("TLS" for a TLS one).

Stands in for the "download server" in the interop test: the sing-box client
sends its XHTTP download GET here, the tap forwards it to the same Xray inbound
the upload goes to (the session lives there), and the log proves which requests
took the download route.

usage: tcp_tap.py LISTEN_PORT TARGET_PORT LOG_FILE
"""
import socket
import sys
import threading


def pump(src, dst):
    try:
        while True:
            data = src.recv(65536)
            if not data:
                break
            dst.sendall(data)
    except OSError:
        pass
    finally:
        for s in (src, dst):
            try:
                s.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass


def handle(client, target_port, log_path):
    try:
        upstream = socket.create_connection(("127.0.0.1", target_port))
    except OSError:
        client.close()
        return
    first = client.recv(65536)
    if first:
        if first[:1] == b"\x16":  # TLS handshake record: the request line is not visible
            line = "TLS"
        else:
            line = first.split(b"\r\n", 1)[0].decode("latin-1", "replace")
        with open(log_path, "a") as log:
            log.write(line + "\n")
        upstream.sendall(first)
    threading.Thread(target=pump, args=(client, upstream), daemon=True).start()
    pump(upstream, client)


def main():
    listen_port, target_port, log_path = int(sys.argv[1]), int(sys.argv[2]), sys.argv[3]
    open(log_path, "w").close()
    server = socket.socket()
    server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    server.bind(("127.0.0.1", listen_port))
    server.listen(64)
    while True:
        client, _ = server.accept()
        threading.Thread(target=handle, args=(client, target_port, log_path), daemon=True).start()


if __name__ == "__main__":
    main()
