import logging
import os
import time
from collections.abc import AsyncGenerator

from sqlalchemy import event
from sqlalchemy.exc import OperationalError, SQLAlchemyError
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine

try:
    from asyncpg.exceptions import PostgresError
except ImportError:
    # If asyncpg is not installed, use only SQLAlchemy exceptions
    PostgresError = type("PostgresError", (Exception,), {})
from dotenv import load_dotenv

from app.metrics import DB_QUERIES_TOTAL, DB_QUERY_ERRORS_TOTAL, DB_QUERY_LATENCY_SECONDS

logger = logging.getLogger(__name__)

load_dotenv()
DATABASE_URL = os.getenv("DATABASE_URL", "postgresql+asyncpg://app:app@db:5432/appdb")

# Create engine with reliability settings
# pool_pre_ping checks connection before use (automatic retry)
# pool_recycle recreates connections every hour to avoid timeout issues
engine = create_async_engine(
    DATABASE_URL,
    echo=False,
    future=True,
    pool_pre_ping=True,  # Check connection before use (automatic retry)
    pool_recycle=3600,  # Recreate connections every hour
    pool_size=5,  # Connection pool size
    max_overflow=10,  # Maximum number of additional connections
    # Settings for proper work with pytest-asyncio
    connect_args={"server_settings": {"application_name": "devops-demo-backend"}},
)
logger.info("Database engine created successfully")
AsyncSessionLocal = async_sessionmaker(engine, class_=AsyncSession, expire_on_commit=False)

# Events on synchronous engine (this is how SQLAlchemy recommends for async)
_sync_engine = engine.sync_engine


@event.listens_for(_sync_engine, "before_cursor_execute")
def _before_cursor_execute(conn, cursor, statement, parameters, context, executemany):
    conn.info.setdefault("query_start_time", []).append(time.perf_counter())


@event.listens_for(_sync_engine, "after_cursor_execute")
def _after_cursor_execute(conn, cursor, statement, parameters, context, executemany):
    start = conn.info.get("query_start_time", []).pop(-1)
    dur = time.perf_counter() - start
    op = _infer_op(statement)
    DB_QUERIES_TOTAL.labels(op=op).inc()
    DB_QUERY_LATENCY_SECONDS.labels(op=op).observe(dur)


@event.listens_for(_sync_engine, "handle_error")
def _handle_error(context):
    op = _infer_op(context.statement or "")
    DB_QUERY_ERRORS_TOTAL.labels(op=op).inc()


def _infer_op(sql: str) -> str:
    s = sql.strip().lower()
    if s.startswith("select"):
        return "select"
    if s.startswith("insert"):
        return "insert"
    if s.startswith("update"):
        return "update"
    if s.startswith("delete"):
        return "delete"
    return "other"


async def get_session() -> AsyncGenerator[AsyncSession, None]:
    """Dependency for getting DB session with error handling"""
    try:
        async with AsyncSessionLocal() as session:
            yield session
    except (OperationalError, PostgresError, SQLAlchemyError) as e:
        logger.error(f"Database error: {type(e).__name__}: {e}")
        raise
    except Exception as e:
        logger.error(f"Unexpected database error: {type(e).__name__}: {e}")
        raise
