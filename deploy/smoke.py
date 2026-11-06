"""Exercise the built server with isolated synthetic data and no upstream access."""
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import tempfile
import time
from urllib.error import HTTPError
from urllib.request import Request, urlopen


def run(binary):
    with tempfile.TemporaryDirectory(prefix="ais2-server-smoke-") as temp:
        root = Path(temp)
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        origin = f"http://127.0.0.1:{port}"
        admin, key = "synthetic-admin-password", "synthetic-model-key"
        (root / ".env").write_text(
            f"ADMIN_AUTH_ENABLED=true\nADMIN_USERNAME=operator\nADMIN_PASSWORD={admin}\n"
            f"PROXY_API_KEY={key}\nAUTO_START=false\nWAA_BACKEND=camoufox\n"
            "PROXY=http://127.0.0.1:1\nREQUEST_BODY_LOG=true\nFAKE_STREAM=true\n",
            encoding="utf-8",
        )
        env = {k: v for k, v in os.environ.items() if k not in {
            "ADMIN_AUTH_ENABLED", "ADMIN_USERNAME", "ADMIN_PASSWORD", "PROXY_API_KEY",
            "AUTO_START", "WAA_BACKEND", "AISTUDIO_AUTH_STATES", "PROXY",
        }}
        process = None

        def call(path, expected, token=None, data=None, method=None):
            headers = {"Authorization": f"Bearer {token}"} if token else {}
            if data is not None:
                headers["Content-Type"] = "application/json"
            request = Request(origin + path, headers=headers, data=None if data is None else json.dumps(data).encode(), method=method)
            try:
                with urlopen(request, timeout=10) as response:
                    status, raw = response.status, response.read()
            except HTTPError as error:
                status, raw = error.code, error.read()
            assert status == expected, (path, status, expected)
            return json.loads(raw) if raw.startswith(b"{") or raw.startswith(b"[") else raw

        def start():
            nonlocal process
            process = subprocess.Popen([str(binary), "-open-ui=false", "-listen", f"127.0.0.1:{port}"], cwd=root, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            for _ in range(100):
                if process.poll() is not None:
                    raise RuntimeError("server stopped during synthetic smoke startup")
                try:
                    call("/health", 200)
                    return
                except OSError:
                    time.sleep(0.1)
            raise TimeoutError("server health timeout")

        def stop():
            if process and process.poll() is None:
                process.send_signal(signal.SIGTERM)
                try:
                    process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    process.kill()
                    raise

        passed = []
        try:
            start()
            call("/v1/models", 401)
            call("/v1/models", 401, admin)
            call("/api/accounts", 401, key)
            call("/api/accounts", 200, admin)
            passed.append("independent_management_and_model_authentication")
            for path in ["/prompts/new_chat", "/generate-speech", "/new_music", "/live", "/library"]:
                call(path, 404)
            for path in ["/v1/responses", "/v1/images/generations", "/v1/live"]:
                call(path, 404, key, {} if path != "/v1/live" else None)
            call("/api/prompts", 404, admin)
            call("/api/accounts/import/browsers", 200, admin)
            call("/api/accounts/import/state", 404, admin, {})
            passed.append("excluded_pages_protocols_and_native_browser_discovery")
            config = call("/api/config", 200, admin)
            assert config["waa_backend"] == "spidermonkey"
            assert not config["request_body_log"] and not config["fake_stream"]
            state = {"cookies": [{"name": name, "value": "synthetic-cookie", "domain": ".google.com", "path": "/", "expires": -1, "secure": True, "httpOnly": True, "sameSite": "Lax"} for name in ["__Secure-1PSID", "SAPISID", "__Secure-1PAPISID", "__Secure-3PAPISID"]], "origins": [], "aistudio2api": {"source": {"browser": "fixture", "email": "fixture@example.com"}}}
            call("/api/accounts/import/cookies/discover", 400, admin, {"cookies": ""})
            cookies = json.dumps(state["cookies"])
            batch = call("/api/accounts/import/cookies", 202, admin, {"items": [
                {"email": "fixture@example.com", "cookies": cookies, "auth_user": 2},
                {"email": "second@example.com", "cookies": cookies, "auth_user": 1},
            ], "locale": "zh-CN", "timezone": "Asia/Hong_Kong"})
            for _ in range(100):
                task = next(item for item in call("/api/accounts/tasks", 200, admin)["tasks"] if item["id"] == batch["task"]["id"])
                if task["state"] != "running":
                    assert task["state"] == "done" and all(item["state"] == "done" for item in task["items"])
                    break
                time.sleep(0.05)
            else:
                raise TimeoutError("synthetic native cookie batch")
            accounts = call("/api/accounts", 200, admin)["accounts"]
            assert len(accounts) == 2 and all(account["enabled"] for account in accounts)
            assert not call("/api/status", 200, admin)["running"]
            passed.append("upstream_cookie_batch_defaults_without_custom_adapter")
            config["proxy_api_key"] = "rotated-synthetic-key"
            call("/api/config", 200, admin, config, "PUT")
            call("/v1/models", 401, key)
            call("/v1/models", 503, "rotated-synthetic-key")
            config["admin_password"] = "new-synthetic-admin"
            config["proxy_api_key"] = admin
            call("/api/config", 400, admin, config, "PUT")
            passed.append("immediate_key_rotation_and_active_password_collision_rejection")
            config = call("/api/config", 200, admin)
            config["proxy_api_key"] = ""
            call("/api/config", 200, admin, config, "PUT")
            call("/v1/models", 401, "rotated-synthetic-key")
            passed.append("empty_model_keys_fail_closed")
            stop()
            assert not (root / "runtime/prompts.db").exists()
            assert not (root / "runtime/states.db").exists()
            assert json.loads((root / "auth/fixture@example.com/account.json").read_text())["auth_user"] == 2
            start()
            accounts = call("/api/accounts", 200, admin)["accounts"]
            assert len(accounts) == 2 and all(account["enabled"] for account in accounts)
            call("/v1/models", 401, "rotated-synthetic-key")
            passed.append("restart_persistence_and_no_unused_databases")
        finally:
            stop()
        print(json.dumps({"passed": passed, "real_model_requests": False, "google_session_probes": False}))


if __name__ == "__main__":
    run(Path(sys.argv[1]).resolve())
