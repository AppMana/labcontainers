from __future__ import annotations

import os
import json
import pathlib
import shutil
import subprocess
import tempfile
from typing import Iterable

import grpc

from . import labcontainers_pb2 as pb
from .rpc import LabcontainersStub

_GRPC_OPTIONS = (
    ("grpc.max_send_message_length", 257 * 1024 * 1024),
    ("grpc.max_receive_message_length", 257 * 1024 * 1024),
)


class Client:
    """A per-test-suite Labcontainers client and child-daemon owner."""

    def __init__(self, socket: str | None = None, *, state_dir: str | None = None, labd: str = "labd"):
        self._temporary = socket is None
        self._directory = tempfile.mkdtemp(prefix="labcontainers-") if self._temporary else None
        self.socket = socket or str(pathlib.Path(self._directory) / "labd.sock")
        if state_dir is None and self._directory is not None:
            state_dir = str(pathlib.Path(self._directory) / "state")
        argv = [labd, "--socket", self.socket, "--parent-pid", str(os.getpid())]
        if state_dir:
            argv += ["--state-dir", state_dir]
        self._process = subprocess.Popen(argv)
        self._channel = grpc.insecure_channel("unix://" + self.socket, options=_GRPC_OPTIONS)
        grpc.channel_ready_future(self._channel).result(timeout=10)
        self._rpc = LabcontainersStub(self._channel)
        self._sessions: dict[str, str] = {}

    @classmethod
    def dial(cls, socket: str) -> Client:
        self = cls.__new__(cls)
        self._temporary = False
        self._directory = None
        self.socket = socket
        self._process = None
        self._channel = grpc.insecure_channel("unix://" + socket, options=_GRPC_OPTIONS)
        grpc.channel_ready_future(self._channel).result(timeout=10)
        self._rpc = LabcontainersStub(self._channel)
        self._sessions = {}
        return self

    def start(self, spec: pb.LabSpec, *, ttl_seconds: int = 7200) -> Session:
        value = self._rpc.CreateSession(pb.CreateSessionRequest(spec=spec, ttl_seconds=ttl_seconds))
        self._sessions[value.id] = value.resume_token
        return Session(self, value)

    @property
    def rpc(self) -> LabcontainersStub:
        """Full transport API, including request fields and gRPC call options."""
        return self._rpc

    def resume(self, session_id: str) -> Session:
        value = self._rpc.GetSession(pb.SessionRef(id=session_id))
        self._sessions[value.id] = value.resume_token
        return Session(self, value)

    def close(self) -> None:
        error: Exception | None = None
        for session_id, token in list(self._sessions.items()):
            try:
                self._rpc.DestroySession(pb.DestroySessionRequest(id=session_id, resume_token=token), timeout=120)
            except Exception as exc:
                if error is None:
                    error = exc
            finally:
                self._sessions.pop(session_id, None)
        self._channel.close()
        if self._process is not None:
            self._process.terminate()
            try:
                self._process.wait(timeout=120)
            except subprocess.TimeoutExpired:
                self._process.kill()
        if self._directory:
            shutil.rmtree(self._directory, ignore_errors=True)
        if error is not None:
            raise error

    def __enter__(self) -> Client:
        return self

    def __exit__(self, *_args) -> None:
        self.close()


class Session:
    def __init__(self, client: Client, value: pb.Session):
        self.client = client
        self.value = value

    @property
    def id(self) -> str:
        return self.value.id

    def node(self, name: str) -> Node:
        return Node(self, name)

    def plan(self, topology: pb.TopologySource | None = None) -> dict:
        """Return native Containerlab ApplyResult JSON; never apply the draft."""
        request = pb.PlanTopologyRequest(session_id=self.id)
        if topology is not None:
            request.topology.CopyFrom(topology)
        return json.loads(self.client.rpc.PlanTopology(request).json)

    def apply(self, topology: pb.TopologySource | None, approved_plan: dict, *, nodes: dict | None = None) -> None:
        """Recheck approved native impact, then reconcile; failures may be partial."""
        self.value = self.client.rpc.ApplyTopology(pb.ApplyTopologyRequest(
            session_id=self.id, topology=topology,
            approved_plan=pb.NativeApplyResult(json=json.dumps(approved_plan).encode()),
            nodes=nodes or {},
        ))

    def keep(self, ttl_seconds: int = 86400) -> None:
        self.value = self.client._rpc.KeepSession(pb.KeepSessionRequest(id=self.id, ttl_seconds=ttl_seconds))
        self.client._sessions.pop(self.id, None)

    def destroy(self) -> None:
        self.client._rpc.DestroySession(pb.DestroySessionRequest(id=self.id, resume_token=self.value.resume_token), timeout=120)
        self.client._sessions.pop(self.id, None)

    def netem(self, node: str, interface: str, **kwargs) -> Fault:
        value = self.client._rpc.ApplyFault(pb.ApplyFaultRequest(session_id=self.id, node=node, interface=interface, netem=pb.Netem(**kwargs)))
        return Fault(self, value)

    def set_link(self, node: str, interface: str, up: bool) -> Fault:
        value = self.client._rpc.ApplyFault(pb.ApplyFaultRequest(session_id=self.id, link_state=pb.LinkState(node=node, interface=interface, up=up)))
        return Fault(self, value)

    def run_timeline(self, actions: Iterable[pb.TimelineAction]) -> pb.TimelineResult:
        return self.client._rpc.RunTimeline(pb.RunTimelineRequest(session_id=self.id, actions=list(actions)))


class Node:
    def __init__(self, session: Session, name: str):
        self.session = session
        self.name = name

    def _ref(self) -> pb.NodeRef:
        return pb.NodeRef(session_id=self.session.id, node=self.name)

    @property
    def ref(self) -> pb.NodeRef:
        """Transport reference for calls through ``client.rpc``."""
        return self._ref()

    def exec(self, *argv: str, stdin: bytes = b"", timeout_seconds: float = 120) -> subprocess.CompletedProcess[bytes]:
        value = self.session.client._rpc.Exec(pb.ExecRequest(node=self._ref(), argv=argv, stdin=stdin, timeout_millis=int(timeout_seconds * 1000)))
        return subprocess.CompletedProcess(argv, value.exit_code, value.stdout, value.stderr)

    def put(self, path: str, content: bytes, mode: int = 0o600) -> None:
        self.session.client._rpc.Put(pb.PutRequest(node=self._ref(), path=path, content=content, mode=mode))

    def crash(self) -> None:
        self._lifecycle(pb.CRASH)

    def power_off(self) -> None:
        self._lifecycle(pb.POWER_OFF)

    def start(self) -> None:
        self._lifecycle(pb.START)

    def restart(self) -> None:
        self._lifecycle(pb.RESTART)

    def prepare_replacement(self, bootstrap: pb.BootstrapData | None = None) -> None:
        """Remove this node and reset its disks; explicitly plan/apply to recreate."""
        self._lifecycle(pb.REPLACE, bootstrap)

    def replace(self, bootstrap: pb.BootstrapData | None = None) -> None:
        """Deprecated alias for prepare_replacement; does not deploy."""
        self.prepare_replacement(bootstrap)

    def _lifecycle(self, action: int, bootstrap: pb.BootstrapData | None = None) -> None:
        request = pb.LifecycleRequest(node=self._ref(), action=action)
        if bootstrap is not None:
            request.bootstrap.CopyFrom(bootstrap)
        self.session.client._rpc.Lifecycle(request, timeout=120)


class Fault:
    def __init__(self, session: Session, value: pb.Fault):
        self.session = session
        self.value = value

    @property
    def id(self) -> str:
        return self.value.id

    def revert(self) -> None:
        self.session.client._rpc.RevertFault(pb.FaultRef(session_id=self.session.id, id=self.id))
