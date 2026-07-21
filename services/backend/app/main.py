import logging
import time
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import Depends, FastAPI, HTTPException, Request, Response
from fastapi.middleware.cors import CORSMiddleware
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession
from starlette.responses import PlainTextResponse

from app import otel
from app.crud import create_item, delete_item, list_items
from app.deps import get_db
from app.grpc_server import start_grpc_server
from app.metrics import FE_WEB_VITAL, HTTP_REQUESTS_TOTAL, REQUEST_LATENCY_SECONDS
from app.schemas import ItemCreate, ItemOut

# Known web-vitals names; the gauge label stays a bounded set by
# construction, anything else from the network is rejected
ALLOWED_WEB_VITALS = {"CLS", "FCP", "INP", "LCP", "TTFB"}

# Logging configuration. trace_id (RFC-0001 D11/ADR-0010): 32 hex chars,
# all zeros when the log line was not emitted inside a span -- see
# app.otel.TraceIdFilter. Still plain text, not JSON (D6 debt, deferred;
# see services/backend/README.md).
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s [trace_id=%(trace_id)s]",
    datefmt="%Y-%m-%d %H:%M:%S",
)
logger = logging.getLogger(__name__)

otel.setup_tracing()
otel.install_trace_id_log_filter()

# Health/metrics endpoints excluded from span creation (D11): healthcheck
# and scrape spam has no diagnostic value and would dominate whatever a
# future Tempo instance samples. Comma-separated regexes matched with
# search() against the full URL -- end-anchored so bare "/metrics" does
# not also swallow the real POST /metrics/frontend web-vitals route.
_OTEL_EXCLUDED_URLS = "/healthz$,/readyz$,/health$,/metrics$"


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    """Starts the gRPC server (:50051) alongside uvicorn; stops it on
    shutdown with a grace period for in-flight WatchItemEvents streams."""
    grpc_server = await start_grpc_server()
    try:
        yield
    finally:
        logger.info("Stopping gRPC server")
        await grpc_server.stop(grace=5)


app = FastAPI(title="DevOps Demo API", lifespan=lifespan)

# CORS for frontend
app.add_middleware(CORSMiddleware, allow_origins=["*"], allow_methods=["*"], allow_headers=["*"])

# W3C trace-context extraction from incoming HTTP headers (D11) -- the
# canary already sends `traceparent` on every journey call.
FastAPIInstrumentor.instrument_app(app, excluded_urls=_OTEL_EXCLUDED_URLS)


def _endpoint_label(request: Request) -> str:
    """Metric label for the endpoint: the matched route template.

    Never the raw URL path -- raw paths (/items/1, /items/2, scanner
    noise) create unbounded label cardinality, and path parameters make
    routes like DELETE /items/{item_id} invisible to SLO rules that
    filter on the endpoint label.
    """
    route = request.scope.get("route")
    return getattr(route, "path", None) or "__unmatched__"


@app.middleware("http")
async def metrics_mw(request: Request, call_next) -> Response:
    """Middleware for HTTP metrics and logging"""
    start = time.time()
    logger.info(f"Request: {request.method} {request.url.path}")
    try:
        response: Response = await call_next(request)
        duration = time.time() - start
        endpoint = _endpoint_label(request)
        REQUEST_LATENCY_SECONDS.labels(request.method, endpoint).observe(duration)
        HTTP_REQUESTS_TOTAL.labels(request.method, endpoint, str(response.status_code)).inc()
        logger.info(f"Response: {request.method} {request.url.path} - {response.status_code} ({duration:.3f}s)")
        return response
    except Exception as e:
        duration = time.time() - start
        logger.error(f"Error: {request.method} {request.url.path} - {type(e).__name__}: {e} ({duration:.3f}s)")
        raise


@app.get("/healthz")
async def healthz() -> dict[str, str]:
    """Liveness check: process is up and serving requests, no dependency checks"""
    return {"status": "ok"}


@app.get("/readyz")
async def readyz(response: Response, db: AsyncSession = Depends(get_db)) -> dict[str, str]:
    """Readiness check: fails if a dependency (database) is unreachable"""
    try:
        await db.execute(select(1))
        return {"status": "ok", "database": "connected"}
    except Exception as e:
        logger.error(f"Readiness check failed: database unreachable: {e}")
        response.status_code = 503
        return {"status": "degraded", "database": "disconnected"}


@app.get("/health")
async def health() -> dict[str, str]:
    """Compatibility alias for /healthz (liveness); dashboards and scripts still target this path"""
    return await healthz()


@app.get("/items", response_model=list[ItemOut])
async def get_items(db: AsyncSession = Depends(get_db)):
    items = await list_items(db)
    # SQLAlchemy 2.0: convert ORM models to schemas
    return [ItemOut(id=i.id, name=i.name) for i in items]


@app.post("/items", response_model=ItemOut, status_code=201)
async def post_item(payload: ItemCreate, db: AsyncSession = Depends(get_db)) -> ItemOut:
    try:
        logger.info(f"Creating item: {payload.name}")
        item = await create_item(db, payload)
        logger.info(f"Item created successfully: id={item.id}, name={item.name}")
        return ItemOut(id=item.id, name=item.name)
    except IntegrityError as e:
        # Uniqueness conflict (duplicate name)
        logger.warning(f"Failed to create item '{payload.name}': already exists")
        raise HTTPException(status_code=409, detail="Item already exists") from e


@app.delete("/items/{item_id}", status_code=204)
async def remove_item(item_id: int, db: AsyncSession = Depends(get_db)) -> Response:
    logger.info(f"Deleting item: id={item_id}")
    ok = await delete_item(db, item_id)
    if not ok:
        logger.warning(f"Item not found: id={item_id}")
        raise HTTPException(status_code=404, detail="Not found")
    logger.info(f"Item deleted successfully: id={item_id}")
    return Response(status_code=204)


# Prometheus endpoint
@app.get("/metrics")
def metrics() -> PlainTextResponse:
    return PlainTextResponse(generate_latest(), media_type=CONTENT_TYPE_LATEST)


# Receive web-vitals reports from the frontend
@app.post("/metrics/frontend")
async def metrics_frontend(req: Request) -> dict[str, bool]:
    try:
        data = await req.json()
    except Exception:
        raise HTTPException(status_code=400, detail="Invalid JSON") from None
    name = data.get("name")
    if name not in ALLOWED_WEB_VITALS:
        raise HTTPException(status_code=400, detail="Unknown metric name")
    try:
        value = float(data.get("value"))
    except (TypeError, ValueError):
        raise HTTPException(status_code=400, detail="Invalid metric value") from None
    FE_WEB_VITAL.labels(name=name).set(value)
    return {"ok": True}
