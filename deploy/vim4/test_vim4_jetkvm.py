import importlib.machinery
import importlib.util
from contextlib import nullcontext
import os
import sys
import fcntl
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

board_spec = importlib.util.spec_from_file_location('vim4media', Path(__file__).resolve().parents[3] / 'board/vim4media.py')
vim4media = importlib.util.module_from_spec(board_spec)
sys.modules['vim4media'] = vim4media
board_spec.loader.exec_module(vim4media)
loader = importlib.machinery.SourceFileLoader('helper', str(Path(__file__).with_name('vim4-jetkvm')))
spec = importlib.util.spec_from_loader(loader.name, loader)
helper = importlib.util.module_from_spec(spec)
loader.exec_module(helper)

# The first JetKVM preset, a valid one-block EDID header and checksum.
EDID = '00FFFFFFFFFFFF00047265058A3F6101101E0104A53420783FC125A8554EA0260D5054BFEF80714F8140818081C081008B009500B300283C80A070B023403020360006442100001A000000FD00304C575716010A202020202020000000FC0042323436574C0A202020202020000000FF0054384E4545303033383532320A01F8'


class GadgetTest(unittest.TestCase):
    def gadget(self, directory, links=('ncm.usb0',)):
        helper.GADGET = Path(directory)
        helper.LUN = helper.GADGET / 'functions/mass_storage.disk0/lun.0'
        helper.LUN.mkdir(parents=True)
        for name in helper.SWITCHABLE:
            (helper.GADGET / 'functions' / name).mkdir(parents=True, exist_ok=True)
        config = helper.GADGET / 'configs/c.1'
        config.mkdir(parents=True)
        for name in links:
            (config / name).symlink_to(helper.GADGET / 'functions' / name)
        (helper.GADGET / 'UDC').write_text('controller')
        for key, value in {'file': '', 'cdrom': '0', 'ro': '1'}.items():
            (helper.LUN / key).write_text(value)
        return config

    def linked(self, config):
        return sorted(p.name for p in config.iterdir() if p.is_symlink())

    def test_mount_replaces_ncm_and_eject_restores_it(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(helper.time, 'sleep'):
            config = self.gadget(directory)
            image = Path(directory) / 'disk.iso'
            image.write_bytes(b'x' * 512)
            fd = os.open(image, os.O_RDONLY)
            try:
                helper.configure_media(fd, cdrom=True)
            finally:
                os.close(fd)
            self.assertEqual(self.linked(config), ['mass_storage.disk0'])
            self.assertEqual((helper.LUN / 'cdrom').read_text(), '1')
            self.assertEqual((helper.LUN / 'ro').read_text(), '1')
            self.assertEqual((helper.GADGET / 'UDC').read_text(), 'controller')
            helper.configure_media(None, cdrom=False)
            self.assertEqual(self.linked(config), ['ncm.usb0'])
            self.assertEqual((helper.LUN / 'file').read_text().strip(), '')

    def test_microphone_link_is_preserved(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(helper.time, 'sleep'):
            config = self.gadget(directory, links=('uac2.usb0',))
            helper.configure_media(None, cdrom=False)
            self.assertEqual(self.linked(config), ['uac2.usb0'])

    def test_changed_media_is_rejected_before_usb_changes(self):
        with tempfile.TemporaryDirectory() as directory:
            config = self.gadget(directory, links=('mass_storage.disk0',))
            (helper.LUN / 'file').write_text('/external.img\n')
            expected = {'file': '', 'ro': True, 'cdrom': False}
            with patch.object(helper.vim4media, 'media_locks', return_value=nullcontext()), \
                    patch.object(helper, 'image_fd', return_value=None), \
                    patch.object(helper, 'relink') as relink:
                for action in ('media-mount', 'media-eject'):
                    with self.subTest(action=action), self.assertRaisesRegex(RuntimeError, 'management panel'):
                        helper.media(action, {'file': 'new.iso', 'cdrom': True, 'expected': expected})
                relink.assert_not_called()
            self.assertEqual((helper.LUN / 'file').read_text(), '/external.img\n')
            self.assertEqual((helper.GADGET / 'UDC').read_text(), 'controller')
            self.assertEqual(self.linked(config), ['mass_storage.disk0'])

    def test_failed_relink_restores_previous_functions(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(helper.time, 'sleep'):
            config = self.gadget(directory)
            def fail():
                raise OSError('busy')
            with self.assertRaises(OSError):
                helper.relink(['mass_storage.disk0'], prepare=fail)
            self.assertEqual(self.linked(config), ['ncm.usb0'])
            self.assertEqual((helper.GADGET / 'UDC').read_text(), 'controller')

    def test_images_must_be_regular_files_inside_the_image_directory(self):
        with tempfile.TemporaryDirectory() as directory:
            helper.IMAGES = Path(directory)
            self.addCleanup(patch.stopall)
            patch.dict(helper.vim4media.IMAGE_ROOTS, jetkvm=helper.IMAGES).start()
            (helper.IMAGES / 'short.img').write_bytes(b'x')
            (helper.IMAGES / 'link.img').symlink_to('/etc/passwd')
            os.mkfifo(helper.IMAGES / 'pipe.img')
            (helper.IMAGES / 'ok.iso').write_bytes(b'x' * 512)
            for name in ('short.img', 'link.img', 'pipe.img', '../ok.iso', '/etc/passwd', '', None):
                with self.subTest(name=name), self.assertRaises((OSError, ValueError)):
                    helper.image_fd(name)
            os.close(helper.image_fd('ok.iso'))


@unittest.skipUnless(os.geteuid() == 0, 'shared lock files require root')
class SharedMediaTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        root = Path(temp.name)
        roots = {source: root / source for source in ('shared', 'jetkvm', 'onekvm', 'pikvm')}
        for directory in roots.values():
            directory.mkdir()
            (directory / 'boot.iso').write_bytes(b'x' * 512)
        data = root / 'data'
        data.mkdir()
        (data / 'work.img').write_bytes(b'x' * 512)
        lun = root / 'lun'
        lun.mkdir()
        for name, value in {'file': '', 'ro': '1', 'cdrom': '0'}.items():
            (lun / name).write_text(value)
        self.addCleanup(patch.stopall)
        patch.multiple(helper.vim4media, IMAGE_ROOTS=roots, DATA_DIR=data, LUN_FILE=lun / 'file',
                       LOCKS=(root / 'lock1', root / 'lock2')).start()
        patch.object(helper, 'LUN', lun).start()

    def mount(self, name, cdrom=False):
        return helper.media('media-mount', {'file': name, 'cdrom': cdrom,
                            'expected': {'file': '', 'ro': True, 'cdrom': False}})

    def test_catalog_and_mount_enforce_read_only_and_effective_mode(self):
        ids = {entry['id'] for entry in helper.OPERATIONS['catalog']({})}
        self.assertTrue({'shared/boot.iso', 'pikvm/boot.iso', 'data/work.img'} <= ids)
        def attach(fd, cdrom):
            self.assertEqual(fcntl.fcntl(fd, fcntl.F_GETFL) & os.O_ACCMODE, os.O_RDONLY)
            for lock in helper.vim4media.LOCKS:
                with lock.open('r+') as contender:
                    with self.assertRaises(BlockingIOError):
                        fcntl.flock(contender, fcntl.LOCK_EX | fcntl.LOCK_NB)
            (helper.LUN / 'file').write_text(os.readlink(f'/proc/self/fd/{fd}'))
        with patch.object(helper, 'configure_media', side_effect=attach):
            mounted = self.mount('shared/boot.iso')
            self.assertTrue(mounted['cdrom'])
            self.assertTrue(mounted['ro'])
            self.assertEqual(mounted['file'], str(helper.vim4media.IMAGE_ROOTS['shared'] / 'boot.iso'))
            (helper.LUN / 'file').write_text('')
            mounted = self.mount('data/work.img', cdrom=True)
            self.assertFalse(mounted['cdrom'])
            self.assertTrue(mounted['ro'])

    def test_edit_lock_refuses_attach_without_usb_changes(self):
        with helper.vim4media.edit_disk('work.img'), patch.object(helper, 'configure_media') as attach:
            with self.assertRaisesRegex(ValueError, '编辑'):
                self.mount('data/work.img')
            attach.assert_not_called()

    def test_rollback_data_disk_retains_validation_and_edit_lock(self):
        disk = helper.vim4media.DATA_DIR / 'work.img'
        (helper.LUN / 'file').write_text(str(disk))
        def relink(links, prepare, restore):
            prepare()
            restore()
            raise OSError('relink failed')
        write_text = Path.write_text
        def write(path, value, *args, **kwargs):
            if path == helper.LUN / 'file' and value.startswith('/proc/self/fd/'):
                fd = int(value.split('/')[-1])
                self.assertEqual(os.fstat(fd).st_ino, disk.stat().st_ino)
                with disk.open('r+') as contender:
                    with self.assertRaises(BlockingIOError):
                        fcntl.flock(contender, fcntl.LOCK_EX | fcntl.LOCK_NB)
            return write_text(path, value, *args, **kwargs)
        with patch.object(helper, 'relink', side_effect=relink), patch.object(Path, 'write_text', write):
            with self.assertRaisesRegex(OSError, 'relink failed'):
                helper.media('media-eject', {'expected': {'file': str(disk), 'ro': True, 'cdrom': False}})


class EdidTest(unittest.TestCase):
    def test_tables_are_validated_and_padded(self):
        self.assertEqual(len(helper.edid_bytes(EDID)), 256)
        for value in ('', 'zz', EDID[:-2], '11' * 128, EDID + '00' * 129, 7):
            with self.subTest(value=value), self.assertRaises(ValueError):
                helper.edid_bytes(value)

    def test_loaded_table_is_not_reloaded_for_the_same_tvserver_run(self):
        with tempfile.TemporaryDirectory() as directory:
            helper.HDMIRX = Path(directory)
            (helper.HDMIRX / 'edid_select').write_text('0x10')
            helper.EDID_LOADED = Path(directory) / 'loaded'
            with patch.object(helper, 'locked'), patch.object(helper, 'tvserver_run', return_value='run1'), \
                    patch.object(helper, 'load_edid') as load:
                self.assertEqual(helper.edid_apply({'edid': EDID}), {'loaded': True})
                self.assertEqual(helper.edid_apply({'edid': EDID}), {'loaded': False})
                self.assertEqual(helper.edid_apply({'edid': EDID, 'force': True}), {'loaded': True})
            self.assertEqual(load.call_count, 2)


class NetworkTest(unittest.TestCase):
    def test_static_ipv4_converts_netmask_and_checks_gateway(self):
        request = {'ipv4_mode': 'static', 'ipv4_static': {'address': '192.168.1.20', 'netmask': '255.255.255.0',
                                                           'gateway': '192.168.1.1', 'dns': ['1.1.1.1']}}
        self.assertEqual(helper.ipv4_properties(request), {
            'ipv4.method': 'manual', 'ipv4.addresses': '192.168.1.20/24', 'ipv4.gateway': '192.168.1.1', 'ipv4.dns': '1.1.1.1'})
        request['ipv4_static']['gateway'] = '10.0.0.1'
        with self.assertRaises(ValueError):
            helper.ipv4_properties(request)
        for bad in ({'ipv4_mode': 'static'}, {'ipv4_mode': 'auto'},
                    {'ipv4_mode': 'static', 'ipv4_static': {'address': '1.2.3.4;reboot', 'netmask': '255.0.0.0'}}):
            with self.subTest(request=bad), self.assertRaises(ValueError):
                helper.ipv4_properties(bad)

    def test_ipv6_modes(self):
        self.assertEqual(helper.ipv6_properties({'ipv6_mode': 'link_local'})['ipv6.method'], 'link-local')
        static = helper.ipv6_properties({'ipv6_mode': 'static', 'ipv6_static': {
            'prefix': '2001:db8::5/64', 'gateway': '2001:db8::1', 'dns': ['2001:4860:4860::8888']}})
        self.assertEqual(static['ipv6.addresses'], '2001:db8::5/64')
        with self.assertRaises(ValueError):
            helper.ipv6_properties({'ipv6_mode': 'static', 'ipv6_static': {'prefix': '2001:db8::5', 'gateway': '2001:db8::1'}})

    def test_failed_reapply_restores_the_profile(self):
        old = {'ipv4.method': 'auto', 'ipv4.addresses': '', 'ipv4.gateway': '', 'ipv4.dns': ''}
        changes, applies = [], []
        def command(*args, **kwargs):
            if args[1:4] == ('--escape', 'no', '-g'):
                return old[args[4]]
            if args[1:3] == ('connection', 'modify'):
                changes.append(dict(zip(args[4::2], args[5::2])))
            if args[1:3] == ('device', 'reapply'):
                applies.append(True)
                if len(applies) == 1:
                    raise RuntimeError('reapply failed')
            return ''
        new = helper.ipv4_properties({'ipv4_mode': 'static', 'ipv4_static': {'address': '10.0.0.2', 'netmask': '255.0.0.0'}})
        with patch.object(helper, 'run', side_effect=command), self.assertRaises(RuntimeError):
            helper.apply_profile('uuid', 'eth0', new)
        self.assertEqual(changes[-1], old)
        self.assertEqual(len(applies), 2)

    def test_hostname_and_mdns_validation(self):
        with patch.object(helper, 'run') as command:
            for name in ('', '-bad', 'a' * 64, 'bad name', 'x;reboot'):
                with self.subTest(name=name), self.assertRaises(ValueError):
                    helper.hostname_set({'hostname': name})
            with self.assertRaises(ValueError):
                helper.mdns_set({'mode': 'everything'})
            command.assert_not_called()


class WifiTest(unittest.TestCase):
    def test_keyfile_keeps_secrets_out_of_arguments(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(helper, 'run') as command:
            helper.NM_CONNECTIONS = Path(directory)
            helper.write_wifi_keyfile('11111111-2222-3333-4444-555555555555', ' café', 'pa\\ss word')
            path = Path(directory) / 'jetkvm-11111111-2222-3333-4444-555555555555.nmconnection'
            text = path.read_text()
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            self.assertIn('id=\\scafé', text)
            self.assertIn('ssid=32;99;97;102;195;169;', text)
            self.assertIn('psk=pa\\\\ss word', text)
            for call in command.call_args_list:
                self.assertNotIn('pa\\ss word', ' '.join(call.args))

    def test_invalid_credentials_are_rejected_before_changes(self):
        with patch.object(helper, 'run') as command:
            for request in ({'ssid': ''}, {'ssid': 'x' * 33}, {'ssid': 'ok', 'password': 'short'},
                            {'ssid': 'ok', 'password': 'line\nbreak1'}):
                with self.subTest(request=request), self.assertRaises(ValueError):
                    helper.wifi_save(request)
            command.assert_not_called()


class TailscaleTest(unittest.TestCase):
    def test_login_server_is_validated_before_commands(self):
        with patch.object(helper.shutil, 'which', return_value='/usr/bin/tailscale'), \
                patch.object(helper, 'run') as command, patch.object(helper.subprocess, 'Popen') as popen:
            for url in ('', 'ftp://x', 'https://host/path', 'https://host;reboot', None):
                with self.subTest(url=url), self.assertRaises(ValueError):
                    helper.tailscale_login({'url': url})
            command.assert_not_called()
            popen.assert_not_called()

    def test_running_node_on_the_same_server_is_left_alone(self):
        state = {('tailscale', 'debug', 'prefs'): '{"ControlURL": ""}',
                 ('tailscale', 'status', '--json'): '{"BackendState": "Running"}'}
        with patch.object(helper.shutil, 'which', return_value='/usr/bin/tailscale'), \
                patch.object(helper, 'run', side_effect=lambda *a, **k: state[a]), \
                patch.object(helper.subprocess, 'Popen') as popen:
            helper.tailscale_login({'url': helper.TAILSCALE_DEFAULT_CONTROL})
            popen.assert_not_called()
            helper.tailscale_login({'url': 'https://headscale.example.com'})
            self.assertIn('--login-server=https://headscale.example.com', popen.call_args.args[0])


class UpdateTest(unittest.TestCase):
    def test_packages_outside_the_state_directory_are_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            package = Path(directory) / 'vim4-jetkvm.deb'
            package.write_bytes(b'deb')
            with patch.object(helper, 'run') as command, self.assertRaises(ValueError):
                helper.update_install({'path': str(package)})
            command.assert_not_called()


class DispatchTest(unittest.TestCase):
    def test_unknown_operations_are_rejected(self):
        for args in ([], ['shell'], ['reboot', 'now']):
            with self.subTest(args=args), self.assertRaises(ValueError):
                helper.main(args)


if __name__ == '__main__':
    unittest.main()
