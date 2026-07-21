import pytest

from app.deps import get_db
from app.main import app


@pytest.mark.asyncio
async def test_healthz(client):
    """Liveness endpoint is always 200, no dependency checks"""
    r = await client.get("/healthz")
    assert r.status_code == 200
    assert r.json() == {"status": "ok"}


@pytest.mark.asyncio
async def test_readyz_ok(client):
    """Readiness endpoint is 200 when the database is reachable"""
    r = await client.get("/readyz")
    assert r.status_code == 200
    data = r.json()
    assert data["status"] == "ok"
    assert data["database"] == "connected"


@pytest.mark.asyncio
async def test_readyz_db_down(client):
    """Readiness endpoint is 503 when the database dependency fails"""

    class _FailingSession:
        async def execute(self, *args, **kwargs):
            raise RuntimeError("db unavailable")

    async def _broken_db():
        return _FailingSession()

    app.dependency_overrides[get_db] = _broken_db
    try:
        r = await client.get("/readyz")
    finally:
        del app.dependency_overrides[get_db]

    assert r.status_code == 503
    data = r.json()
    assert data["status"] == "degraded"
    assert data["database"] == "disconnected"


@pytest.mark.asyncio
async def test_health_alias_still_200(client):
    """/health remains a liveness alias for compatibility with existing dashboards/scripts"""
    r = await client.get("/health")
    assert r.status_code == 200
    assert r.json() == {"status": "ok"}
