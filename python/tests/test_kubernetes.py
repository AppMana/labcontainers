import copy
import json
import subprocess
import unittest
from unittest.mock import Mock, patch

from kubernetes import client as native
import yaml

from labcontainers.kubernetes import apply_objects, write_objects


class KubernetesObjectsTest(unittest.TestCase):
    def pod(self):
        return native.V1Pod(
            api_version="v1", kind="Pod",
            metadata=native.V1ObjectMeta(name="scenario"),
            spec=native.V1PodSpec(containers=[native.V1Container(
                name="test", image="local/test:prepared", image_pull_policy="Never",
                command=["/bin/tool", "literal ; $(not a shell)"],
            )]),
        )

    def test_generated_models_use_upstream_aliases_on_selected_node(self):
        node = Mock()
        node.exec.return_value = subprocess.CompletedProcess([], 0, b"applied", b"")
        pod = self.pod()
        before = copy.deepcopy(pod.to_dict())
        with patch("socket.socket.connect", side_effect=AssertionError("host network used")):
            result = apply_objects(node, pod, kubectl_argv=("kubectl", "--server=https://192.0.2.1:6443"), timeout_seconds=19)
        self.assertEqual(result.stdout, b"applied")
        args, kwargs = node.exec.call_args
        self.assertEqual(args, ("kubectl", "--server=https://192.0.2.1:6443", "apply", "-f", "-"))
        self.assertEqual(kwargs["timeout_seconds"], 19)
        wire = json.loads(kwargs["stdin"])
        self.assertEqual(wire["kind"], "List")
        self.assertEqual(wire["items"][0]["spec"]["containers"][0]["imagePullPolicy"], "Never")
        self.assertNotIn("hostNetwork", wire["items"][0]["spec"])
        self.assertEqual(pod.to_dict(), before)

    def test_custom_resource_fields_and_document_order_survive(self):
        node = Mock()
        custom = {"apiVersion": "test.example/v1", "kind": "Scenario", "spec": {"opaque": "value\n---\nnot a document", "enabled": False}}
        write_objects(node, "/etc/scenario.yaml", self.pod(), custom, mode=0o640)
        args, kwargs = node.put.call_args
        self.assertEqual(args[0], "/etc/scenario.yaml")
        self.assertEqual(kwargs, {"mode": 0o640})
        documents = list(yaml.safe_load_all(args[1]))
        self.assertEqual(len(documents), 2)
        self.assertEqual(documents[1], custom)
        self.assertEqual(documents[0]["metadata"]["name"], "scenario")

    def test_invalid_objects_do_not_reach_node(self):
        for objects in ((), (None,), ({},), (self.pod(), {"kind": "MissingVersion"}),
                        ({"apiVersion": "v1", "kind": "X", "bad": float("nan")},)):
            with self.subTest(objects=objects):
                node = Mock()
                with self.assertRaises((TypeError, ValueError)):
                    apply_objects(node, *objects, kubectl_argv=("kubectl",))
                with self.assertRaises((TypeError, ValueError)):
                    write_objects(node, "/config", *objects)
                node.exec.assert_not_called()
                node.put.assert_not_called()
        for prefix in ((), "kubectl", (None,), ("",)):
            with self.assertRaises(ValueError):
                apply_objects(Mock(), self.pod(), kubectl_argv=prefix)
        for filename in ("", "relative", "/"):
            with self.assertRaises(ValueError):
                write_objects(Mock(), filename, self.pod())

    def test_nonzero_exit_preserves_diagnostics(self):
        node = Mock()
        node.exec.return_value = subprocess.CompletedProcess(["kubectl"], 7, b"partial", b"denied")
        with self.assertRaises(subprocess.CalledProcessError) as caught:
            apply_objects(node, self.pod(), kubectl_argv=("kubectl",))
        self.assertEqual(caught.exception.returncode, 7)
        self.assertEqual(caught.exception.stdout, b"partial")
        self.assertEqual(caught.exception.stderr, b"denied")


if __name__ == "__main__":
    unittest.main()
