from . import labcontainers_pb2 as pb


class LabcontainersStub:
    """Small generated-equivalent gRPC stub kept beside the checked-in protobuf output."""

    def __init__(self, channel):
        service = "/labcontainers.v1.Labcontainers/"
        self.CreateSession = channel.unary_unary(service + "CreateSession", request_serializer=pb.CreateSessionRequest.SerializeToString, response_deserializer=pb.Session.FromString)
        self.GetSession = channel.unary_unary(service + "GetSession", request_serializer=pb.SessionRef.SerializeToString, response_deserializer=pb.Session.FromString)
        self.PlanTopology = channel.unary_unary(service + "PlanTopology", request_serializer=pb.PlanTopologyRequest.SerializeToString, response_deserializer=pb.NativeApplyResult.FromString)
        self.ApplyTopology = channel.unary_unary(service + "ApplyTopology", request_serializer=pb.ApplyTopologyRequest.SerializeToString, response_deserializer=pb.Session.FromString)
        self.DestroySession = channel.unary_unary(service + "DestroySession", request_serializer=pb.DestroySessionRequest.SerializeToString, response_deserializer=pb.Empty.FromString)
        self.KeepSession = channel.unary_unary(service + "KeepSession", request_serializer=pb.KeepSessionRequest.SerializeToString, response_deserializer=pb.Session.FromString)
        self.Exec = channel.unary_unary(service + "Exec", request_serializer=pb.ExecRequest.SerializeToString, response_deserializer=pb.ExecResponse.FromString)
        self.Put = channel.unary_unary(service + "Put", request_serializer=pb.PutRequest.SerializeToString, response_deserializer=pb.Empty.FromString)
        self.Lifecycle = channel.unary_unary(service + "Lifecycle", request_serializer=pb.LifecycleRequest.SerializeToString, response_deserializer=pb.Node.FromString)
        self.ApplyFault = channel.unary_unary(service + "ApplyFault", request_serializer=pb.ApplyFaultRequest.SerializeToString, response_deserializer=pb.Fault.FromString)
        self.RevertFault = channel.unary_unary(service + "RevertFault", request_serializer=pb.FaultRef.SerializeToString, response_deserializer=pb.Empty.FromString)
        self.RunTimeline = channel.unary_unary(service + "RunTimeline", request_serializer=pb.RunTimelineRequest.SerializeToString, response_deserializer=pb.TimelineResult.FromString)
