"""Stage the combat upgrade with public assets and a complete source archive."""
import argparse
import hashlib
import json
import os
import shutil
import subprocess
import tarfile
import zipfile
from pathlib import Path

from release_privacy import check_archive, check_bytes, check_file, check_tree, private_name

ROOT = Path(__file__).resolve().parents[1]
RELEASE_NAME = 'NaiWa-Fight-20261021-r9'
OUT = ROOT / 'Release' / RELEASE_NAME
PET = ROOT / 'Source' / 'NaiWaChatPet'
REPORTS = ROOT / 'reports' / 'fight-upgrade-r9'
SKIP_DIRS = {'.git', '.vs', '.artifacts', 'bin', 'obj', 'target', '__pycache__',
             'node_modules', 'logs', 'ReceivedFiles', 'demo_logs', '.aws', '.codex', '.agents'}
SKIP_SUFFIXES = {'.exe', '.dll', '.pdb', '.zip', '.7z', '.gz', '.log', '.jsonl', '.pprof'}


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def source_files(base):
    for parent, directories, filenames in os.walk(base):
        directories[:] = [d for d in directories if d not in SKIP_DIRS]
        for name in filenames:
            path = Path(parent) / name
            relative = path.relative_to(base)
            if path.suffix.lower() in SKIP_SUFFIXES or private_name(relative.as_posix()):
                continue
            yield path


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--organizer', type=Path, help='Existing or newly built Organizer payload directory')
    args = parser.parse_args()
    OUT.mkdir(parents=True, exist_ok=True)
    REPORTS.mkdir(parents=True, exist_ok=True)
    for name in ('NaiWaChatPet.exe', 'NaiWaFightServer.exe', 'naiwa-fight-server-linux-amd64'):
        if not (OUT / name).is_file():
            raise RuntimeError('Missing freshly built artifact: ' + name)
    shutil.copytree(PET / 'animations', OUT / 'animations', dirs_exist_ok=True)
    organizer = args.organizer or ROOT / 'Release' / 'NaiWaChatPet-20261017-r13' / 'Organizer'
    if not organizer.is_dir() and (OUT / 'Organizer').is_dir():
        organizer = OUT / 'Organizer'
    if not organizer.is_dir():
        raise RuntimeError('Provide an Organizer payload with --organizer, or reuse the unchanged module from the full release')
    if organizer.resolve() != (OUT / 'Organizer').resolve():
        shutil.copytree(organizer, OUT / 'Organizer', dirs_exist_ok=True)
    shutil.copytree(PET / 'source' / 'rig' / 'skilldata', OUT / 'skill-assets', dirs_exist_ok=True)
    shutil.copy2(PET / 'source' / 'play' / 'combat.json', OUT / 'combat.json')
    for name in ('LICENSE', 'NOTICE.md'):
        license_file = ROOT / name
        if not license_file.is_file():
            license_file = ROOT / 'GitHub' / 'NaiWaChatPet-v2.0.1' / name
        shutil.copy2(license_file, OUT / name)
    doc_target = OUT / 'docs'
    doc_target.mkdir(exist_ok=True)
    for name in ('FIGHT-NETWORK.md', 'FIGHT-ANIMATION-INTEGRATION.md', 'FIGHT-TEST-REPORT.md', 'FIGHT-CHANGELOG.md', 'FIGHT-LOBBY-DESIGN.md'):
        shutil.copy2(ROOT / 'docs' / name, doc_target / name)
    shutil.copy2(ROOT / 'docs' / 'FIGHT-README.md', OUT / 'README.md')
    result_target = OUT / 'test-results'
    result_target.mkdir(exist_ok=True)
    for name in ('test-summary.json', 'binary-network-test.json', 'gui-release-candidate.json', 'native-lobby.json', 'lobby-stability.json', 'lobby-open.json', 'focused-checks.json', 'gui-before-clock-change.json', 'gui-clock-improvement.json', 'hit-recovery-check.json'):
        if (REPORTS / name).is_file():
            shutil.copy2(REPORTS / name, result_target / name)
        else:
            (result_target / name).write_text(json.dumps({'status': 'not rerun for this rebuild'}), encoding='utf-8')
    visual_target = OUT / 'previews'
    visual_target.mkdir(exist_ok=True)
    for path in (REPORTS / 'visuals').glob('*'):
        if path.suffix.lower() in ('.png', '.wav'):
            shutil.copy2(path, visual_target / path.name)

    # Verify every embedded user file against the original archive.
    integrity = []
    with tarfile.open(ROOT / 'naiwa-skills-skeletal.tar.gz') as archive:
        for member in archive.getmembers():
            if member.isfile():
                check_bytes(member.name, archive.extractfile(member).read())
        for path in sorted((PET / 'source' / 'rig' / 'skilldata').rglob('*')):
            if not path.is_file():
                continue
            relative = path.relative_to(PET / 'source' / 'rig' / 'skilldata').as_posix()
            original = archive.extractfile('naiwa-skills-export/skeletal/' + relative).read()
            current = path.read_bytes()
            if original != current:
                raise RuntimeError('User asset changed: ' + relative)
            integrity.append({'path': relative, 'sha256': hashlib.sha256(current).hexdigest()})
    (REPORTS / 'asset-integrity.json').write_text(json.dumps({'allByteIdentical': True, 'files': integrity}, indent=2), encoding='utf-8')
    shutil.copy2(REPORTS / 'asset-integrity.json', result_target / 'asset-integrity.json')

    archive_path = ROOT / 'Release' / (RELEASE_NAME + '-Source.zip')
    with zipfile.ZipFile(archive_path, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=6) as archive:
        for base in (PET, ROOT / 'Source' / 'DeskBox-1.5.8'):
            for path in source_files(base):
                name = path.relative_to(ROOT).as_posix()
                check_file(path, name)
                archive.write(path, name)
        for base in (ROOT / 'docs', ROOT / 'scripts'):
            selected = list(base.glob('FIGHT-*.md')) if base.name == 'docs' else [base / 'BUILD_FIGHT.ps1', base / 'package_fight_release.py', base / 'test_fight_release.py', base / 'release_privacy.py', base / 'test_release_privacy.py', base / 'Start-Fight-Server.cmd']
            for path in selected:
                archive.write(path, path.relative_to(ROOT).as_posix())
        for name in ('LICENSE', 'NOTICE.md'):
            archive.write(OUT / name, name)
        for name in ('.gitignore', '.gitattributes'):
            archive.write(ROOT / name, name)
        archive.write(OUT / 'README.md', 'README.md')
        archive.write(ROOT / 'naiwa-skills-skeletal.tar.gz', 'naiwa-skills-skeletal.tar.gz')
    source_count = check_archive(archive_path)

    try:
        tracked = subprocess.check_output(['git', '-c', 'safe.directory=' + ROOT.as_posix(), '-C', str(ROOT), 'rev-parse', 'HEAD'], text=True, stderr=subprocess.DEVNULL).strip()
    except (subprocess.CalledProcessError, FileNotFoundError):
        tracked = None
    manifest = {'gameBuild': '2026.10.21-fighting.9', 'sourceCheckpoint': tracked,
                'sourceArchive': archive_path.name, 'sourceArchiveSHA256': digest(archive_path),
                'sourceArchiveFiles': source_count, 'protocolVersion': 2,
                'userAssetsByteIdentical': True, 'organizer': 'existing r13 module, copied without modifications',
                'executables': {name: digest(OUT / name) for name in ('NaiWaChatPet.exe', 'NaiWaFightServer.exe', 'naiwa-fight-server-linux-amd64')}}
    (OUT / 'release-manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding='utf-8')
    file_count = check_tree(OUT)
    bundle = ROOT / 'Release' / (RELEASE_NAME + '-Windows-x64.zip')
    with zipfile.ZipFile(bundle, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=6) as archive:
        for path in sorted(OUT.rglob('*')):
            if path.is_file():
                archive.write(path, RELEASE_NAME + '/' + path.relative_to(OUT).as_posix())
    check_archive(bundle)
    update = ROOT / 'Release' / (RELEASE_NAME + '-Client-Update.zip')
    with zipfile.ZipFile(update, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=6) as archive:
        archive.write(OUT / 'NaiWaChatPet.exe', 'NaiWaChatPet.exe')
        archive.writestr('UPDATE.txt', '双方请更新到 fighting.9。先退出奶蛙，把本包 EXE 覆盖到原奶蛙目录，保留 animations、skill-assets、Organizer 和个人配置，再重新创建房间。使用独立服务器时也请更新服务器。不要直接在压缩包内运行。\n本版补上持续显示的联机胜利/失败/平局；血量降至原来的80%，标准双方80血，保留原单次伤害；时限改为120秒。改善移动、反向、刹停、技能收招衔接和短输入缓冲，联机预测同步使用实际移动规则。\n'.encode('utf-8'))
    check_archive(update)
    artifacts = [bundle, update, archive_path, OUT / 'NaiWaChatPet.exe', OUT / 'NaiWaFightServer.exe', OUT / 'naiwa-fight-server-linux-amd64']
    checksums = '\n'.join(digest(path) + '  ' + path.relative_to(ROOT / 'Release').as_posix() for path in artifacts) + '\n'
    (ROOT / 'Release' / (RELEASE_NAME + '-SHA256SUMS.txt')).write_text(checksums, encoding='ascii')
    print(json.dumps({'releaseFiles': file_count, 'sourceFiles': source_count, 'bundle': str(bundle), 'source': str(archive_path)}, ensure_ascii=False))


if __name__ == '__main__':
    main()
