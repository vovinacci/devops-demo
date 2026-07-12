from prometheus_client import Counter, Gauge, Histogram

HTTP_REQUESTS_TOTAL = Counter("http_requests_total", "Total HTTP requests", ["method", "endpoint", "status"])
REQUEST_LATENCY_SECONDS = Histogram("http_request_latency_seconds", "Request latency", ["method", "endpoint"])

DB_QUERIES_TOTAL = Counter("db_queries_total", "Total DB queries", ["op"])  # op: select/insert/update/delete/other
DB_QUERY_ERRORS_TOTAL = Counter("db_query_errors_total", "Total DB query errors", ["op"])
DB_QUERY_LATENCY_SECONDS = Histogram(
    "db_query_latency_seconds", "DB query latency seconds", ["op"], buckets=(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2)
)

FE_WEB_VITAL = Gauge("fe_web_vital", "Frontend web vitals metric", ["name"])
