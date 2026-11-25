"""Exercise launcher ownership/rollback without invoking Docker, sudo or systemd."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


FAKE = r'''#!/usr/bin/python3
import json, os, pathlib, sys
command = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
root = pathlib.Path(os.environ['FAKE_STATE'])
state_file = root / 'units.json'
state = json.loads(state_file.read_text()) if state_file.exists() else {}
with (root / 'calls.jsonl').open('a') as log:
    log.write(json.dumps([command, *args]) + '\n')
if command == 'sudo':
    args = [arg for arg in args if arg != '-n']
    if args == ['-v']: sys.exit(0)
    command, args = pathlib.Path(args[0]).name, args[1:]
if command == 'systemctl':
    unit = args[-1]
    if args[0] == 'is-active':
        active = state.get(unit) == 'active'
        if '--quiet' not in args: print('active' if active else 'inactive')
        sys.exit(0 if active else 3)
    if args[0] == 'show': print('loaded' if unit in state else 'not-found')
    if args[0] == 'stop': state.pop(unit, None)
elif command == 'systemd-run':
    unit = next(arg.split('=', 1)[1] for arg in args if arg.startswith('--unit='))
    failed = os.environ.get('FAIL_SERVICE', 'never')
    state[unit] = 'inactive' if unit.endswith('-' + failed + '.service') else 'active'
elif command == 'ss':
    if os.environ.get('BUSY_PORT') and args[-1].endswith(':' + os.environ['BUSY_PORT']): print('LISTEN')
elif command == 'curl':
    if '--unix-socket' in args:
        socket = args[args.index('--unix-socket') + 1]
        name = 'analyzers' if 'analyzers' in socket else 'runner'
    else: name = 'backend' if '8080' in args[-1] else 'frontend'
    sys.exit(0 if any(key.endswith('-' + name + '.service') and value == 'active' for key, value in state.items()) else 7)
elif command == 'docker':
    if 'inspect' in args and os.environ.get('NO_CONTAINER') == '1' and 'container' in args: sys.exit(1)
elif command == 'go':
    pass
elif command == 'ip':
    if 'route' in args and not os.environ.get('NO_DEFAULT_ROUTE'):
        if os.environ.get('VPN_DEFAULT'): print('default dev happ-xray metric 1')
        print('default via 192.168.50.1 dev wlp2s0 metric 100')
    elif 'addr' in args:
        if 'happ-xray' in args: print('7: happ-xray inet 172.19.0.1/30 scope global happ-xray')
        else: print('2: wlp2s0 inet 192.168.50.25/24 brd 192.168.50.255 scope global wlp2s0')
state_file.write_text(json.dumps(state))
'''


class LauncherTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='contest-launcher-')
        self.root = Path(self.temp.name)
        (self.root / 'scripts').mkdir()
        shutil.copy(Path(__file__).resolve().parents[1] / 'dev.sh', self.root / 'scripts/dev.sh')
        shutil.copytree(Path(__file__).resolve().parents[1] / 'lib', self.root / 'scripts/lib')
        (self.root / 'frontend/node_modules').mkdir(parents=True)
        (self.root / 'backend').mkdir()
        commands = self.root / 'commands'
        commands.mkdir()
        for name in ['sudo', 'systemctl', 'systemd-run', 'go', 'npm', 'ss', 'curl', 'sleep', 'ip']:
            path = commands / name
            path.write_text(FAKE)
            path.chmod(0o755)
        self.env = dict(os.environ, PATH=f'{commands}:{os.environ["PATH"]}', FAKE_STATE=str(self.root))

    def tearDown(self):
        self.temp.cleanup()

    def run_launcher(self, command, *arguments, **env):
        return subprocess.run(['bash', str(self.root / 'scripts/dev.sh'), command, *arguments],
                              env=dict(self.env, **env), capture_output=True, text=True, timeout=10)

    def calls(self):
        return [json.loads(line) for line in (self.root / 'calls.jsonl').read_text().splitlines()]

    def test_start_repeat_stop_and_privileges(self):
        for _ in range(2):
            result = self.run_launcher('start')
            self.assertEqual(result.returncode, 0, result.stderr)
        launches = [call for call in self.calls() if 'systemd-run' in call]
        self.assertEqual(len(launches), 4)
        for call in launches:
            unit = next(arg for arg in call if arg.startswith('--unit='))
            expected = '0' if unit.endswith(('-runner.service', '-analyzers.service')) else str(os.getuid())
            self.assertIn('--uid=' + expected, call)
        result = self.run_launcher('stop')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads((self.root / 'units.json').read_text()), {})
        self.assertFalse(any('docker' in ' '.join(call) and any(arg in call for arg in ['stop', 'down', 'rm']) for call in self.calls()))
        self.assertEqual((self.root / 'work/dev/backend.env').stat().st_mode & 0o777, 0o600)

    def test_failure_rolls_back_only_new_units(self):
        result = self.run_launcher('start', FAIL_SERVICE='backend')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(json.loads((self.root / 'units.json').read_text()), {})

    def test_existing_service_survives_partial_failure(self):
        self.assertEqual(self.run_launcher('start').returncode, 0)
        path = self.root / 'units.json'
        state = json.loads(path.read_text())
        path.write_text(json.dumps({key: value for key, value in state.items() if key.endswith('-analyzers.service')}))
        result = self.run_launcher('start', FAIL_SERVICE='backend')
        self.assertNotEqual(result.returncode, 0)
        remaining = json.loads(path.read_text())
        self.assertEqual(len(remaining), 1)
        self.assertTrue(next(iter(remaining)).endswith('-analyzers.service'))

    def test_busy_manual_port_is_not_stopped(self):
        result = self.run_launcher('start', BUSY_PORT='8080')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any('sudo' == call[0] for call in self.calls()))

    def test_lan_address_and_local_backend(self):
        result = self.run_launcher('start', '--lan')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('http://192.168.50.25:5173', result.stdout)
        launch = next(call for call in self.calls() if 'systemd-run' in call and any(arg.endswith('-frontend.service') for arg in call))
        self.assertEqual(launch[launch.index('--host') + 1], '192.168.50.25')
        self.assertIn('LISTEN_HOST="127.0.0.1"', (self.root / 'work/dev/backend.env').read_text())
        self.assertIn('192.168.50.25:5173', self.run_launcher('status').stdout)
        self.assertNotEqual(self.run_launcher('start').returncode, 0)

    def test_lan_without_default_route_and_explicit_address(self):
        result = self.run_launcher('start', '--lan', NO_DEFAULT_ROUTE='1')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('192.168.50.25:5173', result.stdout)
        self.run_launcher('stop')
        result = self.run_launcher('start', '--lan', LAN_IP='10.0.0.12')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('10.0.0.12:5173', result.stdout)

    def test_reject_public_or_invalid_lan_addresses(self):
        for address in ['8.8.8.8', '127.0.0.1', '192.168.1.999', '192.168.01.2', 'bad']:
            result = self.run_launcher('start', '--lan', LAN_IP=address)
            self.assertNotEqual(result.returncode, 0, address)

    def test_router_address_wins_over_vpn_default(self):
        result = self.run_launcher('start', '--lan', VPN_DEFAULT='1')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('192.168.50.25:5173', result.stdout)
        self.assertNotIn('172.19.0.1:5173', result.stdout)

    def test_capacity_is_bounded_and_repeat_start_keeps_it(self):
        self.assertEqual(self.run_launcher('start').returncode, 0)
        state = self.root / 'work/dev/analyzer-sessions'
        selected = int(state.read_text())
        self.assertTrue(1 <= selected <= 6)
        self.assertEqual(self.run_launcher('start').returncode, 0)
        launches = [call for call in self.calls() if 'systemd-run' in call and '--max-sessions' in call]
        self.assertEqual(len(launches), 1)
        self.assertEqual(launches[0][launches[0].index('--max-sessions') + 1], str(selected))

    def test_explicit_invalid_capacity_is_rejected(self):
        for value in ['0', '7', '32', 'many']:
            self.assertNotEqual(self.run_launcher('start', ANALYZER_SESSIONS=value).returncode, 0)

    def test_resource_planner(self):
        helper = self.root / 'scripts/lib/capacity.sh'
        for cores, gib, expected in [(12,25,6),(8,16,4),(4,8,2),(2,4,1),(12,4,1),(12,3,0)]:
            result = subprocess.run(['bash','-c','source "$1"; analyzer_capacity "$2" "$3"','planner',str(helper),str(cores),str(gib*1048576)],capture_output=True,text=True,check=True)
            self.assertEqual(int(result.stdout), expected)


if __name__ == '__main__':
    unittest.main()
