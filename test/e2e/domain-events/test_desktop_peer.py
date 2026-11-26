"""Hermetic receipt-size/deadline and graceful process cleanup regressions."""
import subprocess,types,unittest
from unittest.mock import Mock,patch
from desktop_peer import Peer

class DesktopPeerTests(unittest.TestCase):
    def peer(self):
        peer=Peer.__new__(Peer)
        peer.process=Mock()
        peer.process.stdout.fileno.return_value=7
        return peer

    def test_closed_stdout_is_not_a_success_receipt(self):
        peer=self.peer()
        with patch('desktop_peer.select.select',return_value=([7],[],[])),patch('desktop_peer.os.read',return_value=b''):
            with self.assertRaisesRegex(AssertionError,'exited before receipt'):peer.read()

    def test_receipt_has_a_hard_size_bound(self):
        peer=self.peer()
        with patch('desktop_peer.select.select',return_value=([7],[],[])),patch('desktop_peer.os.read',return_value=b'x'):
            with self.assertRaisesRegex(AssertionError,'oversized'):peer.read()

    def test_silent_peer_has_a_deadline(self):
        peer=self.peer()
        with patch('desktop_peer.time.monotonic',side_effect=[0,36]):
            with self.assertRaisesRegex(AssertionError,'deadline'):peer.read()

    def test_shutdown_closes_input_then_waits_for_persistence(self):
        peer=self.peer()
        peer.process.poll.return_value=None
        peer.stop()
        peer.process.stdin.close.assert_called_once()
        peer.process.wait.assert_called_once_with(timeout=5)
        peer.process.terminate.assert_not_called()

    def test_unresponsive_peer_falls_back_to_bounded_termination(self):
        peer=self.peer()
        peer.process.poll.return_value=None
        peer.process.wait.side_effect=[subprocess.TimeoutExpired('synthetic',5),None]
        peer.stop()
        peer.process.terminate.assert_called_once()
        self.assertEqual(peer.process.wait.call_args_list[-1].kwargs,{'timeout':10})

if __name__=='__main__':unittest.main()
