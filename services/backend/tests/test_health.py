import pytest


@pytest.mark.asyncio
async def test_health(client):
    """Test health check endpoint using fixture client"""
    r = await client.get("/health")
    assert r.status_code == 200
    data = r.json()
    assert data["status"] == "ok"
    assert "database" in data  # Verify that health check checks DB
