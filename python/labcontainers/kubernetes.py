"""Optional native Kubernetes objects over an explicitly selected lab node.

Use the official ``kubernetes.client`` generated models (or native dictionaries
for CRDs). No Kubernetes configuration is loaded on the host and no host API
connection is made. Serialization delegates to the upstream client's codec.
"""

from copy import deepcopy
import json
from pathlib import PurePosixPath
from typing import Sequence

from .client import Node


def _documents(objects: tuple[object, ...]) -> list[dict]:
    if not objects:
        raise ValueError("at least one Kubernetes object is required")
    # Lazy import keeps the vanilla VM/container SDK independent of Kubernetes.
    from kubernetes.client import ApiClient, Configuration

    documents = []
    with ApiClient(configuration=Configuration()) as codec:
        for index, obj in enumerate(objects):
            document = codec.sanitize_for_serialization(deepcopy(obj))
            if not isinstance(document, dict):
                raise TypeError(f"Kubernetes object {index} must be a native model or dictionary")
            for field in ("apiVersion", "kind"):
                if not isinstance(document.get(field), str) or not document[field]:
                    raise ValueError(f"Kubernetes object {index} requires native {field}")
            documents.append(document)
    return documents


def apply_objects(node: Node, *objects: object, kubectl_argv: Sequence[str],
                  timeout_seconds: float = 120):
    """Apply native objects using the explicit node and native kubectl prefix.

    For example ``kubectl_argv=("kubectl", "--kubeconfig", "/etc/admin.conf")``.
    Prefix arguments are passed unchanged; the caller selects credentials,
    endpoint, namespace and flags. Nonzero exits raise the standard
    ``subprocess.CalledProcessError`` with stdout/stderr retained. The native
    CompletedProcess is returned on success. No endpoint probing or retry occurs.
    """
    if isinstance(kubectl_argv, (str, bytes)) or not kubectl_argv or not kubectl_argv[0] or any(
        not isinstance(arg, str) for arg in kubectl_argv
    ):
        raise ValueError("kubectl_argv must be a nonempty sequence of native arguments")
    documents = _documents(objects)
    payload = json.dumps({"apiVersion": "v1", "kind": "List", "items": documents},
                         allow_nan=False).encode("utf-8")
    result = node.exec(*kubectl_argv, "apply", "-f", "-", stdin=payload,
                       timeout_seconds=timeout_seconds)
    result.check_returncode()
    return result


def write_objects(node: Node, filename: str, *objects: object, mode: int = 0o600) -> None:
    """Write native objects at an explicit guest file boundary, without applying.

    JSON documents are valid YAML; multiple objects form a YAML document stream
    for consumers such as kubeadm. No directories or defaults are created. All
    objects serialize before upload, so validation failure cannot truncate the
    guest file. A filename is required only at this infrastructure boundary,
    never as a caller-authored manifest.
    """
    path = PurePosixPath(filename)
    if not path.is_absolute() or str(path) == "/":
        raise ValueError("destination must be an absolute guest file path")
    documents = _documents(objects)
    payload = b"\n---\n".join(json.dumps(doc, allow_nan=False).encode("utf-8")
                                for doc in documents) + b"\n"
    node.put(filename, payload, mode=mode)
