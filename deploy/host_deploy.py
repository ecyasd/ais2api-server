"""Root-owned fixed-repository updater. Mutable data is never restored with code."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import time
from urllib.error import HTTPError
from urllib.request import Request, urlopen

REPOSITORY = "ecyasd/ais2api-server"
ROOT = Path("/srv/ais2api-server")
IMAGE_ENV = Path("/etc/ais2api-server/image.env")
DATA = Path("/var/lib/ais2api-server")
UNIT = "aistudio-api.service"


def run(*arguments):
    result = subprocess.run(arguments, capture_output=True, timeout=180)
    if result.returncode:
        raise RuntimeError("deployment-command-failed")
    return result.stdout.decode().strip()


def get_json(url):
    with urlopen(Request(url, headers={"User-Agent": "ais2api-server-fixed-updater"}), timeout=30) as response:
        return json.load(response)


def digest(path):
    with path.open("rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def download(url, target, expected):
    if target.exists():
        if digest(target) != expected:
            raise ValueError("fixed-artifact-conflict")
        return
    temporary = target.with_name(target.name + ".download")
    with urlopen(url, timeout=60) as source, temporary.open("wb") as output:
        shutil.copyfileobj(source, output, length=1048576)
    temporary.chmod(0o600)
    if digest(temporary) != expected:
        temporary.unlink()
        raise ValueError("artifact-checksum-mismatch")
    os.replace(temporary, target)


def env_value(value):
    if not value:
        return ""
    if value.startswith("'"):
        if len(value) < 2 or not value.endswith("'"):
            raise ValueError("invalid-single-quoted-env")
        return value[1:-1]
    if not value.startswith('"'):
        return value.split(" #", 1)[0].strip()
    if len(value) < 2 or not value.endswith('"'):
        raise ValueError("invalid-double-quoted-env")
    raw, output, index = value[1:-1], bytearray(), 0
    escaped = {"a":7, "b":8, "f":12, "n":10, "r":13, "t":9, "v":11, "\\":92, '"':34}
    while index < len(raw):
        char = raw[index]
        index += 1
        if char != "\\":
            if char in ('"', "\n", "\r"):
                raise ValueError("invalid-env-string")
            output.extend(char.encode("utf-8", errors="surrogateescape"))
            continue
        if index >= len(raw):
            raise ValueError("invalid-env-escape")
        char = raw[index]
        index += 1
        if char in escaped:
            output.append(escaped[char])
        elif char in ("x", "u", "U"):
            size = {"x":2, "u":4, "U":8}[char]
            digits = raw[index:index + size]
            if len(digits) != size or not re.fullmatch(r"[0-9a-fA-F]+", digits):
                raise ValueError("invalid-env-hex")
            number = int(digits, 16)
            index += size
            if char == "x":
                output.append(number)
            else:
                if number > 0x10ffff or 0xd800 <= number <= 0xdfff:
                    raise ValueError("invalid-env-unicode")
                output.extend(chr(number).encode("utf-8"))
        elif char in "01234567":
            digits = char + raw[index:index + 2]
            if len(digits) != 3 or not re.fullmatch(r"[0-7]{3}", digits) or int(digits, 8) > 255:
                raise ValueError("invalid-env-octal")
            output.append(int(digits, 8))
            index += 2
        else:
            raise ValueError("invalid-env-escape")
    return output.decode("utf-8", errors="surrogateescape")


def read_env(path, keys=None):
    result = {}
    for line in path.read_text(encoding="utf-8", errors="surrogateescape").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            raise ValueError("invalid-env-line")
        key, value = line.split("=", 1)
        key = key.strip()
        if keys is None or key in keys:
            result[key] = env_value(value.strip())
    return result


def atomic_write(path, raw):
    descriptor, temporary = tempfile.mkstemp(prefix=".image-", dir=path.parent)
    try:
        os.fchmod(descriptor, 0o600)
        with os.fdopen(descriptor, "wb") as output:
            output.write(raw)
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def verify_health(port):
    values = read_env(DATA / ".env", {"ADMIN_USERNAME", "ADMIN_PASSWORD", "PROXY_API_KEY"})
    admin = values["ADMIN_PASSWORD"]
    keys = [key.strip() for key in values.get("PROXY_API_KEY", "").split(",") if key.strip()]
    origin = f"http://127.0.0.1:{port}"

    def status(path, token=None, cookie=None):
        headers = {"Authorization": "Bearer " + token.encode("utf-8", errors="surrogateescape").decode("latin-1")} if token else {}
        if cookie:
            headers["Cookie"] = cookie
        try:
            with urlopen(Request(origin + path, headers=headers), timeout=5) as response:
                return response.status
        except HTTPError as error:
            return error.code

    for _ in range(90):
        try:
            if status("/health") == 200:
                break
        except OSError:
            pass
        time.sleep(0.5)
    else:
        raise RuntimeError("health-timeout")
    login = Request(origin + "/api/auth/login", method="POST", headers={"Content-Type":"application/json"},
                    data=json.dumps({"username":values["ADMIN_USERNAME"],"password":admin}).encode())
    with urlopen(login, timeout=5) as response:
        session_cookie = response.headers["Set-Cookie"].split(";", 1)[0]
    assert status("/api/accounts", cookie=session_cookie) == 200
    assert status("/api/accounts") == 401
    if admin == admin.strip() and not any(char in admin for char in "\r\n"):
        assert status("/v1/models", admin) == 401
    assert status("/v1/models") == 401
    if keys:
        assert status("/api/accounts", keys[0]) == 401
        assert status("/v1/models", keys[0]) in (200, 503)
    else:
        assert status("/v1/models", "synthetic-denied-token") == 401


def deploy(arguments):
    if not re.fullmatch(r"[0-9a-f]{40}", arguments.commit):
        raise ValueError("invalid-commit")
    if not re.fullmatch(r"[0-9a-f]{64}", arguments.sha256):
        raise ValueError("invalid-checksum")
    if not re.fullmatch(r"server-v[0-9][0-9A-Za-z.+-]{0,79}", arguments.tag):
        raise ValueError("invalid-tag")
    if ROOT.resolve() != ROOT or DATA.resolve() != DATA:
        raise ValueError("unexpected-runtime-path")
    main = get_json(f"https://api.github.com/repos/{REPOSITORY}/git/ref/heads/main")
    if main["object"]["sha"] != arguments.commit:
        raise ValueError("not-reviewed-main")
    release = get_json(f"https://api.github.com/repos/{REPOSITORY}/releases/tags/{arguments.tag}")
    if release.get("draft") or release["target_commitish"] != arguments.commit:
        raise ValueError("release-provenance-mismatch")
    assets = {asset["name"]: asset for asset in release["assets"]}
    assert {"provenance.json", "server-image.tar.gz", "SHA256SUMS"} <= assets.keys()
    prefix = f"https://github.com/{REPOSITORY}/releases/download/{arguments.tag}/"
    provenance = get_json(prefix + "provenance.json")
    if (provenance.get("commit"), provenance.get("repository"), provenance.get("platform"), provenance.get("managedBy")) != (
        arguments.commit, REPOSITORY, "linux/amd64", "ais2api-server"
    ):
        raise ValueError("distribution-provenance-mismatch")
    archive = ROOT / "releases" / arguments.commit / "server-image.tar.gz"
    archive.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    download(prefix + "server-image.tar.gz", archive, arguments.sha256)
    run("docker", "load", "--input", str(archive))
    image = "ais2api-server:" + arguments.commit
    if run("docker", "image", "inspect", image, "--format", "{{.Os}}/{{.Architecture}}") != "linux/amd64":
        raise ValueError("image-platform-mismatch")
    previous = IMAGE_ENV.read_bytes()
    settings = read_env(IMAGE_ENV)
    assert settings["SERVER_DATA"] == str(DATA)
    settings["SERVER_IMAGE"] = image
    new = "".join(key + "=" + value + "\n" for key, value in settings.items()).encode()
    run("systemctl", "stop", UNIT)
    try:
        atomic_write(IMAGE_ENV, new)
        run("systemctl", "start", UNIT)
        verify_health(int(settings.get("SERVER_PORT", "2049")))
    except Exception:
        run("systemctl", "stop", UNIT)
        atomic_write(IMAGE_ENV, previous)
        run("systemctl", "start", UNIT)
        raise
    atomic_write(IMAGE_ENV.with_name("previous-image.env"), previous)
    return {"ok": True, "commit": arguments.commit, "tag": arguments.tag,
            "codeOnly": True, "dataPreserved": True, "modelRequests": 0}


if __name__ == "__main__":
    import fcntl
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--sha256", required=True)
    arguments = parser.parse_args()
    try:
        with Path("/run/ais2api-server-deploy.lock").open("w") as lock:
            fcntl.flock(lock, fcntl.LOCK_EX)
            result = deploy(arguments)
        print(json.dumps(result))
    except Exception as error:
        print(json.dumps({"ok": False, "errorType": type(error).__name__, "detailsWithheld": True}))
        raise SystemExit(1) from None
