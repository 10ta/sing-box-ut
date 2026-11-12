#!/usr/bin/env python3
"""Write the Xray server config and the sing-box client config for one interop scenario.

usage: gen.py OUT_DIR MODE SCENARIO XRAY_BIN
  MODE      stream-up | packet-up
  SCENARIO  plain  cleartext HTTP/1.1, default padding
            full   TLS 1.3 + HTTP/2 + uTLS, VLESS encryption, X-Padding obfs
                   and XMUX: the feature set of the real deployment

Upload goes to 127.0.0.1:6501 (the Xray inbound), download to 127.0.0.1:6502
(tcp_tap.py, which forwards to 6501).
"""
import json
import re
import subprocess
import sys

UUID = "6d0f1a52-3c0e-4f0e-9b1f-5a8f4f0c7d11"
PATH = "/ut/"


def main():
    out_dir, mode, scenario, xray = sys.argv[1:5]
    full = scenario == "full"

    server_xhttp = {"path": PATH, "mode": "auto", "xPaddingBytes": "92-1254"}
    client_xhttp = {"path": PATH, "x_padding_bytes": "92-1254"}
    decryption, encryption = "none", None
    server_stream = {"network": "xhttp", "xhttpSettings": server_xhttp}
    client_tls = None

    if full:
        obfs_server = {
            "xPaddingObfsMode": True,
            "xPaddingKey": "ck",
            "xPaddingHeader": "X-Cache",
            "xPaddingPlacement": "queryInHeader",
            "xPaddingMethod": "tokenish",
        }
        obfs_client = {
            "x_padding_obfs_mode": True,
            "x_padding_key": "ck",
            "x_padding_header": "X-Cache",
            "x_padding_placement": "queryInHeader",
            "x_padding_method": "tokenish",
            "xmux": {
                "max_concurrency": "8-16",
                "h_max_request_times": "600-900",
                "h_max_reusable_secs": "1800-3000",
                "h_keep_alive_period": 30,
            },
        }
        server_xhttp.update(obfs_server)
        client_xhttp.update(obfs_client)

        keys = subprocess.run([xray, "vlessenc"], capture_output=True, text=True, check=True).stdout
        decryption = re.search(r'"decryption": "([^"]+)"', keys).group(1)
        encryption = re.search(r'"encryption": "([^"]+)"', keys).group(1)

        cert = json.loads(
            subprocess.run(
                [xray, "tls", "cert", "--domain=localhost", "--expire=24h"],
                capture_output=True, text=True, check=True,
            ).stdout
        )
        server_stream["security"] = "tls"
        server_stream["tlsSettings"] = {
            "alpn": ["h2", "http/1.1"],
            "certificates": [{"certificate": cert["certificate"], "key": cert["key"]}],
        }
        client_tls = {
            "enabled": True,
            "server_name": "localhost",
            "insecure": True,
            "min_version": "1.3",
            "alpn": ["h2", "http/1.1"],
            "utls": {"enabled": True, "fingerprint": "ios"},
        }

    server = {
        "log": {"loglevel": "debug"},
        "inbounds": [
            {
                "listen": "127.0.0.1",
                "port": 6501,
                "protocol": "vless",
                "settings": {"clients": [{"id": UUID}], "decryption": decryption},
                "streamSettings": server_stream,
            }
        ],
        "outbounds": [
            {
                "protocol": "freedom",
                # Xray blocks loopback targets by default; the test fetches from 127.0.0.1.
                "settings": {"finalRules": [{"action": "allow", "ip": ["127.0.0.0/8"]}]},
            }
        ],
    }

    download = dict(client_xhttp, server="127.0.0.1", server_port=6502)
    if client_tls:
        download["tls"] = client_tls
    outbound = {
        "type": "vless",
        "tag": "out",
        "server": "127.0.0.1",
        "server_port": 6501,
        "uuid": UUID,
        "transport": dict(client_xhttp, type="xhttp", mode=mode, download=download),
    }
    if client_tls:
        outbound["tls"] = client_tls
    if encryption:
        outbound["encryption"] = encryption
    client = {
        "log": {"level": "debug"},
        "inbounds": [{"type": "socks", "tag": "in", "listen": "127.0.0.1", "listen_port": 1080}],
        "outbounds": [outbound],
    }

    for name, config in (("server.json", server), ("client.json", client)):
        with open(f"{out_dir}/{name}", "w") as f:
            json.dump(config, f, indent=2)


if __name__ == "__main__":
    main()
