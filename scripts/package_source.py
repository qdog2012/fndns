import argparse
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile


EXCLUDED_DIRS = {".git", ".tools", "bin", "dist", "node_modules", "__pycache__"}
ROOT_ITEMS = (
    ".gitignore",
    "go.mod",
    "go.sum",
    "PRIVACY.md",
    "README.md",
    "requirements-build.txt",
    "assets",
    "cmd",
    "docs",
    "integration",
    "internal",
    "packaging",
    "scripts",
    "webui",
)


def included(path: Path, root: Path) -> bool:
    return not any(part in EXCLUDED_DIRS for part in path.relative_to(root).parts)


def main() -> None:
    parser = argparse.ArgumentParser(description="创建不含构建依赖和产物的源码归档")
    parser.add_argument("--root", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    root = Path(args.root).resolve()
    output = Path(args.output).resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    with ZipFile(output, "w", compression=ZIP_DEFLATED, compresslevel=9) as archive:
        for item in ROOT_ITEMS:
            source = root / item
            if source.is_file():
                archive.write(source, source.relative_to(root).as_posix())
                continue
            for path in sorted(source.rglob("*")):
                if path.is_file() and included(path, root):
                    archive.write(path, path.relative_to(root).as_posix())
    print(f"Built {output} ({output.stat().st_size / 1024:.1f} KiB)")


if __name__ == "__main__":
    main()
