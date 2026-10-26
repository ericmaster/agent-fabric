#!/usr/bin/env python3
"""Install/remove only the harness-review pointer in explicit native instruction files."""
# Spec: docs/specs/task-retrospective.md
import argparse
import os
from pathlib import Path
import stat
import tempfile

START = b'<!-- harness-review:start -->\n'
END = b'<!-- harness-review:end -->\n'
POINTER = Path(__file__).resolve().parents[1] / 'references/activation.md'


def install(target, remove=False):
    # Spec: docs/specs/task-retrospective.md — per-file atomic, preserve foreign context.
    target = Path(target).absolute()
    if any(path.is_symlink() for path in (target, *target.parents)):
        raise ValueError('instruction file and ancestors must not be symlinks')
    mode = 0o600
    if target.exists():
        info = target.stat()
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_nlink != 1:
            raise ValueError('instruction file must be an owned single-link regular file')
        mode = stat.S_IMODE(info.st_mode)
    original = target.read_bytes() if target.exists() else b''
    if (original.count(START.rstrip()) != original.count(START)
            or original.count(END.rstrip()) != original.count(END)
            or original.count(START) != original.count(END) or original.count(START) > 1
            or (START in original and original.index(START) > original.index(END))):
        raise ValueError('malformed/duplicate managed block; preserve for operator inspection')
    block = START + POINTER.read_bytes().strip() + b'\n' + END
    if START in original:
        start, end = original.index(START), original.index(END) + len(END)
        updated = original[:start] + (b'' if remove else block) + original[end:]
    else:
        # Keep the separator inside the managed block so rollback preserves exact bytes.
        if original and not original.endswith(b'\n'):
            raise ValueError('existing instructions need a trailing newline before installation')
        updated = original if remove else original + block
    if updated == original:
        return
    target.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix='.harness-review-', dir=target.parent)
    try:
        with os.fdopen(fd, 'wb') as output:
            output.write(updated)
            output.flush()
            os.fsync(output.fileno())
            os.fchmod(output.fileno(), mode)
        if (target.read_bytes() if target.exists() else b'') != original:
            raise ValueError('instructions changed during installation; retry after reconciliation')
        os.replace(temporary, target)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def main():
    # Spec: docs/specs/task-retrospective.md via install
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--remove', action='store_true')
    parser.add_argument('files', nargs='+', type=Path)
    args = parser.parse_args()
    try:
        for target in args.files:
            install(target, args.remove)
    except (OSError, ValueError) as error:
        parser.exit(1, f'harness-review context: {error}\n')


if __name__ == '__main__':
    main()
