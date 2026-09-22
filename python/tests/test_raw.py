import unittest

from labcontainers import Client, api
from labcontainers.client import Session


class RawTransportTests(unittest.TestCase):
    def test_apply_current_topology_keeps_native_approval(self):
        class Transport:
            def ApplyTopology(self, request):
                self.request = request
                return api.Session(id="session", state="running")

        client = Client.__new__(Client)
        client._rpc = Transport()
        session = Session(client, api.Session(id="session"))
        approved = {"dry-run": True, "started-nodes": ["vm"]}
        session.apply(None, approved)
        self.assertFalse(client.rpc.request.HasField("topology"))
        self.assertIn(b'"started-nodes": ["vm"]', client.rpc.request.approved_plan.json)
        self.assertEqual(session.value.state, "running")

    def test_raw_client_and_node_reference(self):
        client = Client.__new__(Client)
        client._rpc = object()
        self.assertIs(client.rpc, client._rpc)
        node = Session(client, api.Session(id="session")).node("guest")
        ref = node.ref
        self.assertEqual((ref.session_id, ref.node), ("session", "guest"))
        ref.node = "different"
        self.assertEqual(node.ref.node, "guest")
