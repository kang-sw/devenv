"""Serve downloaded, evaluation-signed workflow artifacts on the smoke host."""

import argparse
import json
import shutil
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit


REPO = "kang-sw/werkschaft"
DOWNLOAD = f"/{REPO}/releases/download/"
RELEASES = f"/repos/{REPO}/releases"
MANIFEST = "werkschaft-tauri-update.json"
ADDR = ("127.0.0.1", 43137)


def read_release(directory, canary):
    with (directory / MANIFEST).open(encoding="utf-8") as source:
        manifest = json.load(source)
    version = manifest["version"]
    if ("-canary." in version) != canary:
        raise ValueError(f"Unexpected channel in {directory}")
    tag = f"v{version}"
    base = f"https://github.com{DOWNLOAD}{tag}/"
    names = {MANIFEST}
    for target in manifest["platforms"].values():
        url = target["url"]
        name = url.removeprefix(base)
        if name == url or name != Path(name).name:
            raise ValueError(f"Unexpected asset URL in {directory}")
        names.update((name, f"{name}.sig"))
    for name in names:
        if not (directory / name).is_file():
            raise ValueError(f"Missing {directory / name}")
    return {
        "tag_name": tag,
        "prerelease": canary,
        "draft": False,
        "assets": [
            {"name": name, "browser_download_url": f"{base}{name}"}
            for name in sorted(names)
        ],
    }, manifest, directory, names


def handler(stable, canaries, fault):
    releases = {release[0]["tag_name"]: release for release in [stable, *canaries]}

    class Handler(BaseHTTPRequestHandler):
        def send_json(self, data):
            body = json.dumps(data).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            path = urlsplit(self.path).path
            if path == f"{RELEASES}/latest":
                self.send_json(stable[0])
                return
            if path == RELEASES:
                self.send_json([release[0] for release in [*canaries, stable]])
                return
            tag, separator, name = path.removeprefix(DOWNLOAD).partition("/")
            if not path.startswith(DOWNLOAD) or not separator or tag not in releases:
                self.send_error(404)
                return
            _, manifest, directory, names = releases[tag]
            if name not in names:
                self.send_error(404)
                return
            if name == MANIFEST:
                platforms = {key: value.copy() for key, value in manifest["platforms"].items()}
                for entry in platforms.values():
                    if fault == "missing-sha":
                        entry.pop("sha256", None)
                    if fault == "bad-signature":
                        entry["signature"] = "invalid"
                self.send_json({**manifest, "platforms": platforms})
                return
            file = directory / name
            self.send_response(200)
            self.send_header("Content-Type", "application/octet-stream")
            self.send_header("Content-Length", str(file.stat().st_size))
            self.end_headers()
            with file.open("rb") as source:
                if fault == "interrupted" and not name.endswith(".sig"):
                    self.wfile.write(source.read(4096))
                    self.close_connection = True
                    return
                if fault == "bad-bytes" and not name.endswith(".sig"):
                    first = source.read(1)
                    self.wfile.write(bytes([first[0] ^ 1]))
                shutil.copyfileobj(source, self.wfile)

    return Handler


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--stable", type=Path, required=True)
    parser.add_argument("--canary", type=Path, action="append", default=[])
    parser.add_argument("--fault", choices=("missing-sha", "bad-signature", "bad-bytes", "interrupted"))
    args = parser.parse_args()
    stable = read_release(args.stable, canary=False)
    canaries = [read_release(directory, canary=True) for directory in args.canary]
    with ThreadingHTTPServer(ADDR, handler(stable, canaries, args.fault)) as server:
        print(f"Serving the Werkschaft evaluation feed at http://{ADDR[0]}:{ADDR[1]}", flush=True)
        server.serve_forever()


if __name__ == "__main__":
    main()
