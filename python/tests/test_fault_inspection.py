import unittest
from labcontainers import api


class FaultInspectionTests(unittest.TestCase):
    def test_generated_session_retains_fault_recovery_metadata(self):
        session = api.Session(id="session", faults=[
            api.Fault(id="link", kind="link-state", node="vm", interface="eth1", active=True, restore_up=False),
            api.Fault(id="netem", kind="netem", node="vm", interface="eth2", active=True),
        ])
        wire = api.Session.FromString(session.SerializeToString())
        self.assertEqual(wire.faults[0].node, "vm")
        self.assertEqual(wire.faults[0].interface, "eth1")
        self.assertTrue(wire.faults[0].HasField("restore_up"))
        self.assertFalse(wire.faults[0].restore_up)
        self.assertFalse(wire.faults[1].HasField("restore_up"))
