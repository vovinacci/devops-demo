import pytest_asyncio
from httpx import ASGITransport, AsyncClient
from sqlalchemy.ext.asyncio import AsyncSession

from app.db import AsyncSessionLocal, engine
from app.main import app
from app.seed import default_items, reset_items, seed_items


@pytest_asyncio.fixture(scope="function", autouse=True)
async def _db_seed_session():
    """Fixture for preparing DB before each test"""
    async with AsyncSessionLocal() as db:
        await reset_items(db)
        await seed_items(db, default_items(10, deterministic=True))
        await db.commit()

    yield
    # Cleanup not needed - each test has its own setup


@pytest_asyncio.fixture(scope="function", autouse=True)
async def _cleanup_db_pool():
    """Fixture for cleaning up DB after each test

    Deletes all items created during the test (including seed items).
    """
    yield

    # Clean up all items after test
    async with AsyncSessionLocal() as db:
        try:
            await reset_items(db)
        except Exception as e:
            # Ignore errors during cleanup
            await db.rollback()
            import warnings

            warnings.warn(f"Failed to cleanup items: {e}", stacklevel=2)


@pytest_asyncio.fixture(scope="session", autouse=True)
async def _dispose_engine_pool():
    """Dispose the shared engine pool once, after the whole session.

    Runs on the session-scoped event loop (see pyproject pytest config),
    the same loop every pooled connection was created on. The previous
    per-test dispose existed only to survive per-test loops.
    """
    yield
    await engine.dispose()


@pytest_asyncio.fixture
async def client():
    """Fixture for HTTP client"""
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as ac:
        yield ac


@pytest_asyncio.fixture
async def db() -> AsyncSession:
    """Fixture for DB session"""
    async with AsyncSessionLocal() as session:
        yield session
        # Context manager automatically closes session
