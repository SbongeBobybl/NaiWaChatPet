import unittest
from release_privacy import check_bytes
class ReleasePrivacyTests(unittest.TestCase):
    def test_runtime_files_and_secrets_rejected(self):
        for name, content in [('Organizer/companion-memory.dpapi', b'encrypted'), ('Organizer/agent.json', b'{}'), ('payload.json', b'{"api_key":"fixture-private"}'), ('app.exe', b'key=' + b'sk-' + b'a1'*16)]:
            with self.subTest(name=name), self.assertRaises(RuntimeError): check_bytes(name, content)
    def test_clean_defaults_allowed(self):
        check_bytes('defaults.json', b'{"api_key":"","model":"deepseek"}')
        check_bytes('app.exe', b'locale-lk-sk-' + b'locale'*50)
    def test_weapon_memory_and_backup_state_rejected(self):
        for name in ['play.json', 'play.json.bak', 'settings.json.tmp', 'logs/requests.jsonl',
                     'companion-memory.dpapi.old', 'companion-preferences.json.backup',
                     'ReceivedFiles/image.png', 'demo_logs/data.txt']:
            with self.subTest(name=name), self.assertRaises(RuntimeError):
                check_bytes(name, b'{}')
    def test_utf16_key_rejected(self):
        with self.assertRaises(RuntimeError):
            check_bytes('binary.dat', ('sk-' + 'ab'*16).encode('utf-16-le'))
if __name__ == '__main__': unittest.main()
