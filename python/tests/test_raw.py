import unittest

from labcontainers import Client, api
from labcontainers.client import Session


class RawTransportTests(unittest.TestCase):
    def test_raw_client_and_node_reference(self):
        client = Client.__new__(Client)
        client._rpc = object()
        self.assertIs(client.rpc, client._rpc)
        node = Session(client, api.Session(id="session")).node("guest")
        ref = node.ref
        self.assertEqual((ref.session_id, ref.node), ("session", "guest"))
        ref.node = "different"
        self.assertEqual(node.ref.node, "guest")
