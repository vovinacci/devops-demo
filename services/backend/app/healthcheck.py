"""Container healthcheck: HTTP /healthz liveness + gRPC Health Checking
Protocol (RFC-0001 D6 -- both must pass). Invoked as `python -m
app.healthcheck` from the Dockerfile HEALTHCHECK, never imported by the
app itself.
"""

import os
import sys
import urllib.request

import grpc
from grpc_health.v1 import health_pb2, health_pb2_grpc

GRPC_PORT = int(os.getenv("GRPC_PORT", "50051"))


def main() -> int:
    try:
        # both checks run sequentially inside the Docker healthcheck's 10s
        # budget -- keep the sum of timeouts safely below it
        urllib.request.urlopen("http://localhost:8000/healthz", timeout=3)
    except Exception:
        return 1

    try:
        channel = grpc.insecure_channel(f"localhost:{GRPC_PORT}")
        stub = health_pb2_grpc.HealthStub(channel)
        resp = stub.Check(health_pb2.HealthCheckRequest(service=""), timeout=3)
    except Exception:
        return 1
    return 0 if resp.status == health_pb2.HealthCheckResponse.SERVING else 1


if __name__ == "__main__":
    sys.exit(main())
