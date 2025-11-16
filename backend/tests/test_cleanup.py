import pytest


@pytest.mark.asyncio
async def test_cleanup_verification(client):
    """Test verifies that items created in previous test were cleaned up"""
    # After previous test everything should be cleaned up (including seed items)
    # But before this test _db_seed_session will create seed items again
    r = await client.get("/items")
    assert r.status_code == 200
    data = r.json()

    # Should be exactly 10 seed items (created before this test)
    assert len(data) == 10

    # Verify that TestCleanupItem doesn't exist (was deleted by cleanup fixture)
    item_names = [item["name"] for item in data]
    assert "TestCleanupItem" not in item_names


@pytest.mark.asyncio
async def test_multiple_items_cleanup(client):
    """Test verifies cleanup of multiple created items"""
    # Get initial count
    r = await client.get("/items")
    assert r.status_code == 200
    initial_count = len(r.json())

    # Create several items
    created_ids = []
    for name in ["Item1", "Item2", "Item3"]:
        r = await client.post("/items", json={"name": name})
        assert r.status_code == 201
        created_ids.append(r.json()["id"])

    # Verify that all were created
    r = await client.get("/items")
    assert r.status_code == 200
    assert len(r.json()) == initial_count + 3

    # After test completion all 3 items should be automatically deleted
