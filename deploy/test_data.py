import json
from contextlib import closing
from pathlib import Path
import sqlite3
import tempfile
import unittest

from data import backup, restore, verify


class DataLifecycle(unittest.TestCase):
    def test_consistent_backup_latest_cookie_and_separate_restore(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            live = root / "live"
            account = live / "auth/fixture@example.com"
            account.mkdir(parents=True)
            cookie = account / "storage-state.json"
            cookie.write_text(json.dumps({"cookies": [{"name": "SID", "value": "latest-synthetic-cookie"}]}))
            runtime = live / "runtime"
            runtime.mkdir()
            with closing(sqlite3.connect(runtime / "requests.db")) as database:
                database.execute("PRAGMA journal_mode=WAL")
                database.execute("CREATE TABLE usage (count INTEGER)")
                database.execute("INSERT INTO usage VALUES (7)")
                database.commit()
                backup(live, root / "backup", True)
            restore(root / "backup", root / "restored")
            self.assertEqual((root / "restored/auth/fixture@example.com/storage-state.json").read_bytes(), cookie.read_bytes())
            with closing(sqlite3.connect(root / "restored/runtime/requests.db")) as database:
                self.assertEqual(database.execute("SELECT count FROM usage").fetchone()[0], 7)
            cookie.write_text("newer-synthetic-cookie")
            with self.assertRaises(ValueError):
                restore(root / "backup", live)
            self.assertEqual(cookie.read_text(), "newer-synthetic-cookie")

    def test_refuse_active_snapshot_and_corrupt_backup(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            live = root / "live"
            live.mkdir()
            (live / ".env").write_text("synthetic")
            with self.assertRaises(ValueError):
                backup(live, root / "backup", False)
            backup(live, root / "backup", True)
            (root / "backup/.env").write_text("changed")
            with self.assertRaises(ValueError):
                verify(root / "backup")


if __name__ == "__main__":
    unittest.main()
