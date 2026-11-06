"""Exercise code rollback against isolated data, without Docker/AWS/Google."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import MagicMock, patch


class NativeEnvTests(unittest.TestCase):
    def test_native_env_values_preserve_valid_credentials(self):
        import host_deploy as deployer
        with tempfile.TemporaryDirectory() as temporary:
            file = Path(temporary) / ".env"
            file.write_text(" ADMIN_PASSWORD = alpha\\beta # comment\nPROXY_API_KEY=key#literal\nADMIN_USERNAME = ' spaced value ' \nIGNORED=\"unclosed\n", encoding="utf-8")
            values = deployer.read_env(file, {"ADMIN_PASSWORD", "PROXY_API_KEY", "ADMIN_USERNAME"})
            self.assertEqual(values, {"ADMIN_PASSWORD": "alpha\\beta", "PROXY_API_KEY": "key#literal", "ADMIN_USERNAME": " spaced value "})
            self.assertEqual(deployer.env_value(r'"quote\" line\n\a\v\x41\101\u4e2d\U0001f600"'), 'quote" line\n\a\vAA中😀')

    def test_health_check_uses_native_default_when_username_is_omitted(self):
        self.assert_health_username("", "admin")

    def test_health_check_preserves_custom_username(self):
        self.assert_health_username("ADMIN_USERNAME=synthetic-owner\n", "synthetic-owner")

    def assert_health_username(self, username_line, expected_username):
        import host_deploy as deployer
        with tempfile.TemporaryDirectory() as temporary:
            data = Path(temporary)
            (data / ".env").write_text(
                username_line + "ADMIN_PASSWORD=synthetic-password\nPROXY_API_KEY=synthetic-model-key\n",
                encoding="utf-8",
            )
            logins = []

            def http(request, timeout):
                response = MagicMock()
                response.__enter__.return_value = response
                path = request.full_url.removeprefix("http://127.0.0.1:2049")
                token = request.get_header("Authorization")
                cookie = request.get_header("Cookie")
                response.status = 401
                if path == "/health":
                    response.status = 200
                elif path == "/api/auth/login":
                    logins.append(json.loads(request.data))
                    response.status = 200
                    response.headers = {"Set-Cookie": "synthetic-session=ok; HttpOnly"}
                elif path == "/api/accounts" and cookie == "synthetic-session=ok":
                    response.status = 200
                elif path == "/v1/models" and token == "Bearer synthetic-model-key":
                    response.status = 200
                return response

            with patch.object(deployer, "DATA", data), patch.object(deployer, "urlopen", http):
                deployer.verify_health(2049)
            self.assertEqual(logins, [{"username": expected_username, "password": "synthetic-password"}])


@unittest.skipUnless(os.name == "posix", "host updater is Linux-only")
class HostDeliveryTests(unittest.TestCase):
    def test_failed_upgrade_rolls_back_only_image_and_keeps_latest_cookie(self):
        import host_deploy as deployer
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            data = root / "data"
            data.mkdir()
            cookie = data / "storage-state.json"
            cookie.write_text("previous-synthetic-cookie")
            image_env = root / "image.env"
            previous = f"SERVER_IMAGE=ais2api-server:previous\nSERVER_DATA={data}\nSERVER_PORT=2049\n".encode()
            image_env.write_bytes(previous)
            commit = "a" * 40
            archive = root / "releases" / commit / "server-image.tar.gz"
            archive.parent.mkdir(parents=True)
            archive.write_bytes(b"synthetic-image-archive")
            arguments = argparse.Namespace(commit=commit, tag="server-v0.1.0", sha256=hashlib.sha256(archive.read_bytes()).hexdigest())
            commands = []

            def metadata(url):
                if "/git/ref/" in url:
                    return {"object": {"sha": commit}}
                if "/releases/tags/" in url:
                    return {"draft": False, "target_commitish": commit, "assets": [{"name": name} for name in ("provenance.json", "server-image.tar.gz", "SHA256SUMS")]}
                return {"commit": commit, "repository": deployer.REPOSITORY, "platform": "linux/amd64", "managedBy": "ais2api-server"}

            def command(*values):
                commands.append(values)
                return "linux/amd64" if values[:3] == ("docker", "image", "inspect") else ""

            def failed_health(_):
                self.assertIn(commit, image_env.read_text())
                cookie.write_text("newest-synthetic-cookie")
                raise RuntimeError("synthetic-upgrade-failure")

            with patch.object(deployer, "ROOT", root), patch.object(deployer, "DATA", data), patch.object(deployer, "IMAGE_ENV", image_env), patch.object(deployer, "get_json", metadata), patch.object(deployer, "run", command), patch.object(deployer, "verify_health", failed_health):
                with self.assertRaises(RuntimeError):
                    deployer.deploy(arguments)
            self.assertEqual(image_env.read_bytes(), previous)
            self.assertEqual(cookie.read_text(), "newest-synthetic-cookie")
            self.assertEqual([item[1] for item in commands if item[0] == "systemctl"], ["stop", "start", "stop", "start"])

    def test_invalid_tag_cannot_reach_network_or_service_control(self):
        import host_deploy as deployer
        with patch.object(deployer, "get_json") as metadata, patch.object(deployer, "run") as command:
            for tag in ("../release", "server-v1;anything", "https://example.invalid", "server-v1\nother"):
                with self.assertRaises(ValueError):
                    deployer.deploy(argparse.Namespace(commit="a" * 40, sha256="b" * 64, tag=tag))
            metadata.assert_not_called()
            command.assert_not_called()
