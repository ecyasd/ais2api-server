"""Offline consistent backup/verification/restore; never prints credential data.

Stop the service before snapshotting account files. SQLite databases use SQLite's
backup API. Restore only into a new empty directory; ordinary code rollback never
invokes restore and continues using the latest credential files.
"""
import argparse
from contextlib import closing
import hashlib
import json
from pathlib import Path
import shutil
import sqlite3


def digest(path):
    with path.open("rb") as file:
        return hashlib.file_digest(file, "sha256").hexdigest()


def empty_target(path):
    if path.exists() and (not path.is_dir() or any(path.iterdir())):
        raise ValueError("destination must be a new or empty directory")
    path.mkdir(parents=True, exist_ok=True, mode=0o700)


def backup(source, destination, stopped):
    source, destination = source.resolve(), destination.resolve()
    if not stopped:
        raise ValueError("stop the service before backing up account files")
    if source == destination or source in destination.parents:
        raise ValueError("backup must be outside the live data directory")
    if not source.is_dir():
        raise ValueError("source data directory does not exist")
    empty_target(destination)
    files = []
    for path in sorted(source.rglob("*")):
        if path.is_symlink():
            raise ValueError("data symlinks require an explicit migration inventory")
        if not path.is_file() or ".leases" in path.parts or path.name.endswith(("-wal", "-shm", ".tmp")):
            continue
        relative = path.relative_to(source)
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        if path.suffix == ".db":
            with closing(sqlite3.connect(path.as_uri() + "?mode=ro", uri=True)) as original:
                with closing(sqlite3.connect(target)) as copied:
                    original.backup(copied)
        else:
            shutil.copyfile(path, target)
        target.chmod(0o600)
        files.append({"path": relative.as_posix(), "sha256": digest(target)})
    manifest = {"format": "ais2-server-backup-v1", "files": files}
    manifest_path = destination / "backup-manifest.json"
    manifest_path.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    manifest_path.chmod(0o600)
    verify(destination)
    return len(files)


def verify(source):
    source = source.resolve()
    manifest = json.loads((source / "backup-manifest.json").read_text(encoding="utf-8"))
    if manifest.get("format") != "ais2-server-backup-v1":
        raise ValueError("unsupported backup format")
    paths = set()
    for item in manifest["files"]:
        relative = Path(item["path"])
        path = (source / relative).resolve()
        if relative.is_absolute() or source not in path.parents or item["path"] in paths or path.is_symlink():
            raise ValueError("invalid backup path")
        paths.add(item["path"])
        if digest(path) != item["sha256"]:
            raise ValueError("backup integrity verification failed")
        if path.suffix == ".db":
            with closing(sqlite3.connect(path.as_uri() + "?mode=ro", uri=True)) as database:
                if database.execute("PRAGMA quick_check").fetchone()[0] != "ok":
                    raise ValueError("backup SQLite consistency verification failed")
    return manifest


def restore(source, destination):
    source, destination = source.resolve(), destination.resolve()
    if source == destination or source in destination.parents or destination in source.parents:
        raise ValueError("restore destination must be separate from backup")
    manifest = verify(source)
    empty_target(destination)
    for item in manifest["files"]:
        target = destination / item["path"]
        target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        shutil.copyfile(source / item["path"], target)
        target.chmod(0o600)
    return len(manifest["files"])


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["backup", "verify", "restore"])
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--destination", type=Path)
    parser.add_argument("--service-stopped", action="store_true")
    args = parser.parse_args()
    if args.action != "verify" and args.destination is None:
        parser.error("--destination is required")
    if args.action == "backup":
        count = backup(args.source, args.destination, args.service_stopped)
    elif args.action == "restore":
        count = restore(args.source, args.destination)
    else:
        count = len(verify(args.source)["files"])
    print(json.dumps({"ok": True, "action": args.action, "files": count, "credential_values_printed": False}))
