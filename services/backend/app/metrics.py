from prometheus_client import Counter, Gauge, Histogram

HTTP_REQUESTS_TOTAL = Counter("http_requests_total", "Total HTTP requests", ["method", "endpoint", "status"])
# buckets must include the 0.2 SLO boundary (slo:latency:ratio7d filters
# le="0.2"; the default client buckets jump 0.1 -> 0.25)
REQUEST_LATENCY_SECONDS = Histogram(
    "http_request_latency_seconds",
    "Request latency",
    ["method", "endpoint"],
    buckets=(0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.25, 0.5, 1, 2.5, 5, 10),
)

DB_QUERIES_TOTAL = Counter("db_queries_total", "Total DB queries", ["op"])  # op: select/insert/update/delete/other
DB_QUERY_ERRORS_TOTAL = Counter("db_query_errors_total", "Total DB query errors", ["op"])
DB_QUERY_LATENCY_SECONDS = Histogram(
    "db_query_latency_seconds", "DB query latency seconds", ["op"], buckets=(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2)
)

FE_WEB_VITAL = Gauge("fe_web_vital", "Frontend web vitals metric", ["name"])

# gRPC RED metrics (RFC-0001 D6/D3). grpc_method is the bare RPC name --
# ItemService's fixed 3-RPC surface, bounded cardinality by construction.
# For streaming RPCs (WatchItemEvents), handled_total/handling_seconds
# cover the whole stream lifetime (subscribe to disconnect), not
# per-message; grpc_server_stream_messages_total counts individual
# messages pushed on the stream.
GRPC_SERVER_HANDLED_TOTAL = Counter("grpc_server_handled_total", "Total gRPC calls completed, by method and status code", ["grpc_method", "grpc_code"])
GRPC_SERVER_HANDLING_SECONDS = Histogram(
    "grpc_server_handling_seconds",
    "gRPC call duration seconds (streaming: subscribe-to-disconnect lifetime)",
    ["grpc_method"],
    buckets=(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
)
GRPC_SERVER_STREAM_MESSAGES_TOTAL = Counter("grpc_server_stream_messages_total", "Total messages sent on streaming gRPC methods", ["grpc_method"])
