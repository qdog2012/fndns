import argparse
import hashlib
import io
import os
from pathlib import Path
import shutil
import tarfile
import tempfile


EXECUTABLE_NAMES = {
    "main", "install_init", "install_callback", "upgrade_init", "upgrade_callback",
    "config_init", "config_callback", "uninstall_init", "uninstall_callback", "fndns",
}


def add_tree(archive: tarfile.TarFile, source: Path, prefix: str = "") -> None:
    for path in sorted(source.rglob("*")):
        relative = path.relative_to(source).as_posix()
        arcname = f"{prefix}/{relative}".strip("/")
        info = archive.gettarinfo(str(path), arcname)
        info.uid = info.gid = 0
        info.uname = info.gname = "root"
        info.mtime = 0
        if path.is_dir():
            info.mode = 0o755
            archive.addfile(info)
        else:
            info.mode = 0o755 if path.name in EXECUTABLE_NAMES or relative.startswith("cmd/") else 0o644
            with path.open("rb") as handle:
                archive.addfile(info, handle)


def build_tar(source: Path, output: Path) -> None:
    output.parent.mkdir(parents=True, exist_ok=True)
    with tarfile.open(output, "w:gz", format=tarfile.PAX_FORMAT) as archive:
        add_tree(archive, source)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", required=True)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--version", required=True)
    parser.add_argument("--platform", required=True, choices=("x86", "arm"))
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    root = Path(args.root).resolve()
    binary = Path(args.binary).resolve()
    output = Path(args.output).resolve()
    with tempfile.TemporaryDirectory(prefix="fndns-fpk-") as temp_name:
        temp = Path(temp_name)
        source_root = root / "packaging" / "fnos"
        app_root = temp / "app"
        (app_root / "bin").mkdir(parents=True)
        (app_root / "bin" / "fndns").write_bytes(binary.read_bytes())
        # FNOS 在运行时从 TRIM_APPDEST/ui 读取桌面入口。外层 ui/ 仅供
        # 安装器注册使用，因此必须在 app.tgz 中再保留一份。
        shutil.copytree(source_root / "ui", app_root / "ui")
        app_tgz = temp / "app.tgz"
        build_tar(app_root, app_tgz)
        checksum = hashlib.md5(app_tgz.read_bytes()).hexdigest()

        package_root = temp / "package"
        for path in source_root.rglob("*"):
            relative = path.relative_to(source_root)
            # 与 FNOS 已发布应用一致，桌面 UI 只放在 app.tgz 的 ui/ 下。
            if relative.parts and relative.parts[0] == "ui":
                continue
            destination = package_root / relative
            if path.is_dir():
                destination.mkdir(parents=True, exist_ok=True)
            else:
                destination.parent.mkdir(parents=True, exist_ok=True)
                destination.write_bytes(path.read_bytes())
        (package_root / "app.tgz").write_bytes(app_tgz.read_bytes())
        manifest_path = package_root / "manifest"
        manifest = manifest_path.read_text(encoding="utf-8")
        replacements = {"version": args.version, "platform": args.platform, "checksum": checksum}
        lines = []
        for line in manifest.splitlines():
            key = line.split("=", 1)[0].strip() if "=" in line else ""
            if key in replacements:
                line = f"{key:<16}= {replacements[key]}"
            lines.append(line)
        manifest_path.write_text("\n".join(lines) + "\n", encoding="utf-8", newline="\n")
        build_tar(package_root, output)
        print(f"Built {output} ({output.stat().st_size / 1024 / 1024:.1f} MiB)")


if __name__ == "__main__":
    main()
