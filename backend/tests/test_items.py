import pytest


@pytest.mark.asyncio
async def test_items_crud(client):
    """Test CRUD operations using fixture client from conftest.py"""
    # list empty (or with seed data)
    r = await client.get("/items")
    assert r.status_code == 200
    initial_count = len(r.json())

    # create
    r = await client.post("/items", json={"name": "Book"})
    assert r.status_code == 201
    assert r.json()["name"] == "Book"
    item_id = r.json()["id"]

    # conflict - attempt to create duplicate
    r = await client.post("/items", json={"name": "Book"})
    assert r.status_code == 409

    # list - should be one more
    r = await client.get("/items")
    assert r.status_code == 200
    data = r.json()
    assert len(data) == initial_count + 1

    # delete
    r = await client.delete(f"/items/{item_id}")
    assert r.status_code == 204

    # not found delete
    r = await client.delete(f"/items/{item_id}")
    assert r.status_code == 404


@pytest.mark.asyncio
async def test_cleanup_created_items(client):
    """Test verifies that created items are automatically cleaned up after test"""
    # Get initial count of items (seed items)
    r = await client.get("/items")
    assert r.status_code == 200
    initial_count = len(r.json())
    assert initial_count == 10  # Should be 10 seed items

    # Create new item
    r = await client.post("/items", json={"name": "TestCleanupItem"})
    assert r.status_code == 201
    created_item_id = r.json()["id"]

    # Verify that item was created
    r = await client.get("/items")
    assert r.status_code == 200
    data = r.json()
    assert len(data) == initial_count + 1
    assert any(item["id"] == created_item_id for item in data)

    # After test completion fixture _cleanup_db_pool will automatically delete this item
    # Next test will verify that cleanup works
