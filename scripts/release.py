"""Build distribution from explicit fresh outputs; never collect user profiles."""
import argparse
import hashlib
import json
import shutil
import struct
import subprocess
import sys
import zipfile
from pathlib import Path
sys.dont_write_bytecode = True
from release_privacy import check_archive, check_file, check_tree, private_name

ROOT = Path(__file__).resolve().parent.parent
TAG = 'v2.0.1'
VERSION = '2.0.1'
SKIP = {'bin', 'obj', 'target', '.git', '.artifacts', 'artifacts', 'apppackages',
        'node_modules', '.vs', '__pycache__', '.cache', 'release', 'build'}

def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def source_entries():
    for p in sorted(ROOT.rglob('*')):
        rel = p.relative_to(ROOT)
        if any(part.lower() in SKIP for part in rel.parts) or not p.is_file():
            continue
        if p.is_symlink():
            raise RuntimeError('Source link: ' + str(rel))
        if private_name(rel.as_posix()):
            raise RuntimeError('Unexpected private source file: ' + str(rel))
        if p.suffix.lower() in {'.pdb', '.pyc', '.exp', '.lib'}:
            continue
        if p.suffix.lower() in {'.exe', '.dll'} and 'thirdparty' not in {s.lower() for s in rel.parts} and 'third_party' not in rel.parts:
            continue
        if p.stat().st_size >= 100 * 1024 * 1024:
            raise RuntimeError('Source file exceeds GitHub 100 MiB limit: ' + str(rel))
        yield p, rel.as_posix()

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--payload', type=Path, required=True)
    parser.add_argument('--pet', type=Path, required=True)
    parser.add_argument('--iscc', type=Path, required=True)
    args = parser.parse_args()
    release = ROOT / 'Release'
    ready = release / ('NaiWaChatPet-' + TAG)
    archive = release / ('NaiWaChatPet-Source-' + TAG + '.zip')
    setup = release / ('NaiWaChatPet-Setup-' + TAG + '-x64.exe')
    if any(p.exists() for p in [ready, archive, setup]):
        raise SystemExit('A release already exists; archive it before rebuilding.')
    for required in [args.pet, args.iscc, args.payload / 'DeskBox.exe', args.payload / 'DeskBox.InstallManifest.txt']:
        if not required.is_file():
            raise SystemExit('Missing required build input: ' + str(required))
    for forbidden in ['DeskBox.dll', 'coreclr.dll', 'hostfxr.dll']:
        if (args.payload / forbidden).exists():
            raise SystemExit('Expected Native AOT retail output, found ' + forbidden)
    data = args.pet.read_bytes()[:4096]
    pe = struct.unpack_from('<I', data, 0x3c)[0]
    if struct.unpack_from('<H', data, pe + 24 + 68)[0] != 2:
        raise SystemExit('Pet must use Windows GUI subsystem')
    check_file(args.pet)
    print('Payload privacy:', check_tree(args.payload), flush=True)
    entries = list(source_entries())
    for path, name in entries:
        check_file(path, name)
    ready.mkdir(parents=True)
    shutil.copy2(args.pet, ready / 'NaiWaChatPet.exe')
    shutil.copytree(args.payload, ready / 'Organizer')
    for name in ['README.md', 'CHANGELOG.md', 'LICENSE', 'NOTICE.md']:
        shutil.copy2(ROOT / name, ready / name)
    shutil.copytree(ROOT / 'docs', ready / 'docs')
    (ready / 'licenses').mkdir()
    for package in ['go-winio-0.6.2', 'sys-0.10.0']:
        shutil.copy2(ROOT / 'Source/NaiWaChatPet/third_party' / package / 'LICENSE', ready / 'licenses' / (package + '.txt'))
    shutil.copy2(ROOT / 'LICENSE', ready / 'Organizer/LICENSE')
    source_hash = hashlib.sha256()
    for path, name in entries:
        source_hash.update(name.encode('utf-8') + b'\0' + bytes.fromhex(digest(path)))
    (ready / 'release.json').write_text(json.dumps({
        'version': VERSION, 'tag': TAG, 'sourceTreeSha256': source_hash.hexdigest(),
        'petSha256': digest(ready / 'NaiWaChatPet.exe'),
        'organizerSha256': digest(ready / 'Organizer/DeskBox.exe'),
    }, indent=2), encoding='utf-8')
    hashes = {p.relative_to(ready).as_posix(): digest(p) for p in sorted(ready.rglob('*')) if p.is_file()}
    (ready / 'checksums.json').write_text(json.dumps(hashes, indent=2), encoding='utf-8')
    print('Distribution privacy:', check_tree(ready), flush=True)
    with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED, compresslevel=6) as z:
        for path, name in entries:
            z.write(path, name)
    print('Source privacy:', check_archive(archive), flush=True)
    subprocess.run([str(args.iscc), '/DDeskBoxNativeAot=1', '/DDeskBoxBundledRuntime=1', str(ROOT/'installer/NaiWaChatPet.iss')], check=True)
    if not setup.is_file():
        raise SystemExit('Installer not produced')
    # Scan compiled bytes as an additional check; input tree is the authoritative privacy gate.
    check_file(setup)
    (release / 'SHA256SUMS.txt').write_text('\n'.join(digest(p) + '  ' + p.name for p in [setup, archive]) + '\n', encoding='utf-8')
    print('Release complete:', release)

if __name__ == '__main__':
    main()
