"""Fail closed on private runtime data or credentials; never print secret values."""
from pathlib import Path
import json, re, zipfile

PRIVATE_PARTS = {'private-config', 'deskbox-agent', 'chat-history', 'user-data', '.env', '.aws', 'logs', '.codex', '.agents', '__pycache__'}
PRIVATE_NAMES = {'companion-prefs.json', 'companion-preferences.json', 'companion-memory.dpapi.tmp', 'companion-memory.dpapi', 'agent.json', 'settings.json', 'chat-history.json', 'cookies', 'credentials.json', 'play.json', 'requests.jsonl', 'recovery.json', 'vcvars-env.txt'}
SECRET = re.compile(rb'(?:(?<![A-Za-z0-9_-])sk-[A-Za-z0-9_-]{20,160}(?![A-Za-z0-9_-])|-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----)')
FIELDS = {'apikey', 'api_key', 'access_token', 'refresh_token', 'secretkey', 'password'}

def private_name(name):
    p = Path(name.replace('\\', '/'))
    parts = {s.lower() for s in p.parts}
    base = p.name.lower()
    private_base = any(base == n or base.startswith(n + '.') for n in PRIVATE_NAMES)
    return bool(parts & PRIVATE_PARTS) or private_base or p.suffix.lower() in {'.dpapi', '.pfx', '.pem', '.key', '.log', '.jsonl'} or base.startswith('.env.')

def check_bytes(name, data):
    if private_name(name):
        raise RuntimeError('Private runtime file in release: ' + name)
    if SECRET.search(data) or SECRET.search(data.replace(b'\x00', b'')):
        raise RuntimeError('Possible credential in release: ' + name)
    if Path(name).suffix.lower() == '.json':
        try: value = json.loads(data)
        except (ValueError, UnicodeError): return
        def inspect(v):
            if isinstance(v, dict):
                for key, item in v.items():
                    if key.lower() in FIELDS and item not in ('', None, False):
                        raise RuntimeError('Nonempty secret field in release: ' + name)
                    inspect(item)
            elif isinstance(v, list):
                for item in v: inspect(item)
        inspect(value)

def check_file(path, name=None):
    path = Path(path)
    check_bytes(name or path.name, path.read_bytes())

def check_tree(root):
    root = Path(root)
    count = 0
    for p in root.rglob('*'):
        if p.is_symlink(): raise RuntimeError('Links are not allowed in release: ' + str(p.relative_to(root)))
        if p.is_file():
            check_file(p, p.relative_to(root).as_posix()); count += 1
    return count

def check_archive(path):
    with zipfile.ZipFile(path) as archive:
        seen = set()
        for item in archive.infolist():
            name = item.filename.replace('\\', '/')
            if name.startswith('/') or ':' in name or '..' in name.split('/') or name.casefold() in seen:
                raise RuntimeError('Unsafe or duplicate archive entry: ' + name)
            seen.add(name.casefold())
            if not item.is_dir(): check_bytes(item.filename, archive.read(item))
        return len(archive.infolist())

if __name__ == '__main__':
    import sys
    for name in sys.argv[1:]:
        p = Path(name)
        count = check_tree(p) if p.is_dir() else check_archive(p)
        print('Privacy gate passed:', p.name, count, 'files')
