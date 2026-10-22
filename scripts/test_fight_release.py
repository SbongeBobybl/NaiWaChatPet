"""Exercise the newly built server executable using real TCP clients."""
import argparse

import hashlib
import json
import socket
import subprocess
import time
from pathlib import Path


class Peer:
    def __init__(self, address):
        self.socket = socket.create_connection(address, timeout=5)
        self.socket.settimeout(8)
        self.reader = self.socket.makefile('rb')

    def send(self, kind, **fields):
        self.socket.sendall(json.dumps(dict(version=2, type=kind, **fields)).encode() + b'\n')

    def receive(self, predicate):
        deadline = time.monotonic() + 8
        while time.monotonic() < deadline:
            line = self.reader.readline(128 << 10)
            if not line:
                raise RuntimeError('Server disconnected')
            update = json.loads(line)
            if predicate(update):
                return update
        raise RuntimeError('Update timeout')

    def close(self):
        self.reader.close()
        self.socket.close()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('server', type=Path)
    parser.add_argument('--report', type=Path, required=True)
    args = parser.parse_args()
    creation = getattr(subprocess, 'CREATE_NO_WINDOW', 0)
    proc = subprocess.Popen([str(args.server.resolve()), '-listen', '127.0.0.1:0'],
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            text=True, creationflags=creation)
    peers = []
    start = time.monotonic()
    try:
        line = proc.stdout.readline().strip()
        endpoint = line.rsplit(' ', 1)[-1]
        host, port = endpoint.rsplit(':', 1)
        address = (host, int(port))
        a, b = Peer(address), Peer(address)
        peers += [a, b]
        a.send('create')
        welcome = a.receive(lambda u: u['type'] == 'welcome')
        room, token = welcome['room'], welcome['token']
        b.send('join', room=room)
        other = b.receive(lambda u: u['type'] == 'welcome')
        assert token != other['token'], 'reconnect tokens must be isolated'
        a.send('ready')
        b.send('ready')
        a.receive(lambda u: u.get('state') == 'fighting')
        b.receive(lambda u: u.get('state') == 'fighting')
        a.send('input', seq=1, input={'FanPressed': True})
        b.send('input', seq=1, input={'FanPressed': True})
        seen_a, seen_b = {}, {}
        for peer, seen in [(a, seen_a), (b, seen_b)]:
            while True:
                update = peer.receive(lambda u: u.get('snapshot') is not None)
                snap = update['snapshot']
                seen[snap['Tick']] = snap
                if all(f['HP'] < 100 for f in snap['Fighters']):
                    break
        matched = sorted(set(seen_a) & set(seen_b))
        assert matched, 'clients never observed the same authoritative tick'
        shared = matched[-1]
        for key in ('Fighters', 'Projectiles', 'Clock', 'Phase'):
            assert seen_a[shared][key] == seen_b[shared][key], key
        b.send('input', seq=1, input={'PowerPressed': True})
        b.close()
        a.receive(lambda u: u.get('state') == 'disconnected')
        resumed = Peer(address)
        peers.append(resumed)
        resumed.send('resume', room=room, token=other['token'])
        resumed.receive(lambda u: u['type'] == 'welcome')
        a.receive(lambda u: u.get('state') == 'fighting')
        invalid = Peer(address)
        peers.append(invalid)
        invalid.socket.sendall(b'{"version":999,"type":"create"}\n')
        assert invalid.receive(lambda u: u['type'] == 'error')['error'] == 'version_mismatch'
        invalid.socket.sendall(b'{"version":1,"type":"damage","hp":99999}\n')
        assert invalid.receive(lambda u: u['type'] == 'error')['error'] == 'invalid_message'
        resumed.send('leave')
        a.receive(lambda u: u['type'] == 'left')
        result = {'passed': True, 'transport': 'real TCP loopback against newly built executable',
                  'serverSHA256': hashlib.sha256(args.server.read_bytes()).hexdigest(),
                  'durationSeconds': time.monotonic() - start,
                  'checks': ['create', 'join', 'ready', 'countdown', 'simultaneous fan skills',
                             'same-tick HP/MP/state equality', 'disconnect', 'resume',
                             'protocol version rejection', 'forged damage rejection', 'leave'],
                  'crossMachineVerified': False, 'publicDeploymentVerified': False}
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding='utf-8')
        print(json.dumps(result, ensure_ascii=False))
    finally:
        for peer in peers:
            try:
                peer.close()
            except OSError:
                pass
        proc.terminate()
        try:
            proc.communicate(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.communicate()


if __name__ == '__main__':
    main()
