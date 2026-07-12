import pytest


@pytest.mark.asyncio
async def test_seeded_items_present(client):
    r = await client.get("/items")
    assert r.status_code == 200
    data = r.json()
    assert len(data) >= 10
