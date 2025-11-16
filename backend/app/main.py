import logging
import time

from fastapi import Depends, FastAPI, HTTPException, Request, Response
from fastapi.middleware.cors import CORSMiddleware
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession
from starlette.responses import PlainTextResponse

from app.crud import create_item, delete_item, list_items
from app.deps import get_db
from app.metrics import HTTP_REQUESTS_TOTAL, REQUEST_LATENCY_SECONDS
from app.schemas import ItemCreate, ItemOut

# Logging configuration
logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(name)s - %(levelname)s - %(message)s", datefmt="%Y-%m-%d %H:%M:%S")
logger = logging.getLogger(__name__)

app = FastAPI(title="DevOps Demo API")

# CORS for frontend
app.add_middleware(CORSMiddleware, allow_origins=["*"], allow_methods=["*"], allow_headers=["*"])


# Middleware for HTTP metrics and logging
@app.middleware("http")
async def metrics_mw(request: Request, call_next) -> Response:
    start = time.time()
    logger.info(f"Request: {request.method} {request.url.path}")
    try:
        response: Response = await call_next(request)
        duration = time.time() - start
        REQUEST_LATENCY_SECONDS.labels(request.method, request.url.path).observe(duration)
        HTTP_REQUESTS_TOTAL.labels(request.method, request.url.path, str(response.status_code)).inc()
        logger.info(f"Response: {request.method} {request.url.path} - {response.status_code} ({duration:.3f}s)")
        return response
    except Exception as e:
        duration = time.time() - start
        logger.error(f"Error: {request.method} {request.url.path} - {type(e).__name__}: {e} ({duration:.3f}s)")
        raise


@app.get("/health")
async def health(db: AsyncSession = Depends(get_db)) -> dict[str, str]:
    """Health check endpoint with database connection check"""
    try:
        # Check database connection
        from sqlalchemy import select

        await db.execute(select(1))
        return {"status": "ok", "database": "connected"}
    except Exception as e:
        logger.error(f"Health check failed: {e}")
        return {"status": "degraded", "database": "disconnected"}


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


# Receive web-vitals from frontend (if added)
@app.post("/metrics/frontend")
async def metrics_frontend(req: Request) -> dict[str, bool]:
    data = await req.json()
    from app.metrics import FE_WEB_VITAL  # import here to avoid cycles

    name = str(data.get("name"))
    value = float(data.get("value", 0))
    FE_WEB_VITAL.labels(name=name).set(value)
    return {"ok": True}
