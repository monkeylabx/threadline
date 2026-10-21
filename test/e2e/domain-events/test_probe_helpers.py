"""Hermetic regression tests for timeout, redaction and child cleanup contracts."""
import subprocess
import unittest
from unittest.mock import Mock,call,patch
import worker_outage as probe

class ProbeHelpersTest(unittest.TestCase):
    def test_completed_child_is_not_signalled(self):
        child=Mock()
        child.poll.return_value=0
        probe.stop(child)
        child.terminate.assert_not_called()
        child.kill.assert_not_called()

    def test_cooperative_child_is_terminated_and_reaped(self):
        child=Mock()
        child.poll.return_value=None
        probe.stop(child)
        child.terminate.assert_called_once_with()
        child.wait.assert_called_once_with(timeout=10)
        child.kill.assert_not_called()

    def test_unresponsive_child_is_killed_then_reaped(self):
        child=Mock()
        child.poll.return_value=None
        child.wait.side_effect=[subprocess.TimeoutExpired('test-child',10),0]
        probe.stop(child)
        child.terminate.assert_called_once_with()
        child.kill.assert_called_once_with()
        self.assertEqual(child.wait.call_args_list,[call(timeout=10),call(timeout=5)])

    def test_database_timeout_and_failure_are_bounded_and_redacted(self):
        secret_command=['psql','postgres://user:secret@fixture/db']
        errors=[subprocess.TimeoutExpired(secret_command,5),subprocess.CalledProcessError(1,secret_command)]
        for error in errors:
            with self.subTest(error=type(error).__name__),patch.object(probe.subprocess,'check_output',side_effect=error) as execute:
                with self.assertRaises(RuntimeError) as result:probe.sql('SELECT 1')
                self.assertNotIn('secret',str(result.exception))
                self.assertEqual(execute.call_args.kwargs['timeout'],5)

    def test_version_probe_has_its_own_deadline(self):
        with patch.object(probe.subprocess,'check_output',return_value='nats-server: wrong') as execute:
            with self.assertRaises(ValueError):probe.prepare('unused',Mock())
            self.assertEqual(execute.call_args.kwargs['timeout'],5)

if __name__=='__main__':unittest.main()
