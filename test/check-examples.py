#!/usr/bin/env python3
"""Check the client examples: both READMEs carry them unchanged, and sing-box accepts them.

usage: check-examples.py SING_BOX_BIN

The files in EXAMPLES must equal, comments aside, the ```jsonc blocks of
README.md and of README.zh-CN.md, in the same order (the comments are
translated there, the JSON is not). Each example then goes through
`sing-box check` as the only outbound (masque-client: the only endpoint), with
the placeholders filled in and "encryption" dropped (a real key needs Xray).
The WARP keys are throwaway test keys, valid only in form.
"""
import json
import os
import re
import subprocess
import sys
import tempfile

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
EXAMPLES = [
    "examples/sing-box-client-minimal.jsonc",
    "examples/sing-box-client.jsonc",
    "examples/sing-box-warp-minimal.jsonc",
    "examples/sing-box-warp.jsonc",
]
PLACEHOLDERS = {
    "<UUID>": "6d0f1a52-3c0e-4f0e-9b1f-5a8f4f0c7d11",
    # throwaway P-256 keys: base64 SEC 1 private key, base64 PKIX public key
    "<WARP_PRIVATE_KEY>": "MHcCAQEEIFQPygDRmSDFoghcxXYuNbFtwBc/WZ3P+9DHDx1q/2zloAoGCCqGSM49AwEHoUQDQgAErFOg1jEr3tDtrgK/"
    "MSeCHv+RZeXi3MP+lc+iu0bHTiot3NwkaRFsoItNJJPKNFy9ePSYnIaldDfwkhGd4d98jQ==",
    "<WARP_PUBLIC_KEY>": "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEgJBU1bUD3K2eIJv7IBTRzEic91cazAPswEl2uvv/yAemdWFwrrTOj/"
    "jpX9qFEcjb4h2lh9qcvcNEaYwsC3gb2g==",
}
READMES = ["README.md", "README.zh-CN.md"]


def strip_comments(text):
    """Drop // comments outside strings (sing-box accepts them, json does not)."""
    out, i, in_string = [], 0, False
    while i < len(text):
        c = text[i]
        if in_string:
            out.append(c)
            if c == "\\":
                out.append(text[i + 1])
                i += 1
            elif c == '"':
                in_string = False
        elif c == '"':
            in_string = True
            out.append(c)
        elif text.startswith("//", i):
            while i < len(text) and text[i] != "\n":
                i += 1
            continue
        else:
            out.append(c)
        i += 1
    return "".join(out)


def load(text, where):
    try:
        return json.loads(strip_comments(text))
    except json.JSONDecodeError as e:
        sys.exit(f"FAIL: {where}: {e}")


def main():
    singbox = sys.argv[1]
    examples = []
    for path in EXAMPLES:
        with open(os.path.join(ROOT, path)) as f:
            examples.append(load(f.read(), path))
    for readme in READMES:
        with open(os.path.join(ROOT, readme)) as f:
            blocks = re.findall(r"```jsonc\n(.*?)```", f.read(), re.S)
        if len(blocks) != len(EXAMPLES):
            sys.exit(f"FAIL: {readme}: {len(blocks)} jsonc blocks, want {len(EXAMPLES)}")
        for i, block in enumerate(blocks):
            if load(block, f"{readme} block {i + 1}") != examples[i]:
                sys.exit(f"FAIL: {readme} block {i + 1} differs from {EXAMPLES[i]}")
        print(f"ok: {readme} matches the examples")
    for path, outbound in zip(EXAMPLES, examples):
        text = json.dumps(outbound)
        for placeholder, value in PLACEHOLDERS.items():
            text = text.replace(placeholder, value)
        outbound = json.loads(text)
        outbound.pop("encryption", None)
        section = "endpoints" if outbound["type"] == "masque-client" else "outbounds"
        with tempfile.NamedTemporaryFile("w", suffix=".json", delete=False) as f:
            json.dump({"log": {"level": "error"}, section: [outbound]}, f)
        result = subprocess.run([singbox, "check", "-c", f.name], capture_output=True, text=True)
        os.unlink(f.name)
        if result.returncode != 0:
            sys.exit(f"FAIL: sing-box check {path}:\n{result.stdout}{result.stderr}")
        print(f"ok: sing-box check {path}")
    print("PASS")


if __name__ == "__main__":
    main()
