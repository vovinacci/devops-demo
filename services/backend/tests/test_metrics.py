import pytest


@pytest.mark.asyncio
async def test_web_vital_accepted(client):
    resp = await client.post("/metrics/frontend", json={"name": "LCP", "value": 1234.5})
    assert resp.status_code == 200
    assert resp.json() == {"ok": True}

    metrics = (await client.get("/metrics")).text
    assert 'fe_web_vital{name="LCP"} 1234.5' in metrics


@pytest.mark.asyncio
async def test_web_vital_unknown_name_rejected(client):
    resp = await client.post("/metrics/frontend", json={"name": "EVIL" * 10, "value": 1})
    assert resp.status_code == 400

    metrics = (await client.get("/metrics")).text
    assert "EVIL" not in metrics


@pytest.mark.asyncio
async def test_web_vital_bad_value_rejected(client):
    resp = await client.post("/metrics/frontend", json={"name": "CLS", "value": "not-a-number"})
    assert resp.status_code == 400

    resp = await client.post("/metrics/frontend", json={"name": "CLS"})
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_web_vital_invalid_json_rejected(client):
    resp = await client.post("/metrics/frontend", content=b"not json", headers={"Content-Type": "application/json"})
    assert resp.status_code == 400


@pytest.mark.asyncio
async def test_endpoint_label_uses_route_template(client):
    # 404 on a real route: still the template, not the raw path
    resp = await client.delete("/items/999999")
    assert resp.status_code == 404

    metrics = (await client.get("/metrics")).text
    assert 'endpoint="/items/{item_id}"' in metrics
    assert 'endpoint="/items/999999"' not in metrics


@pytest.mark.asyncio
async def test_endpoint_label_bounded_for_unmatched_paths(client):
    resp = await client.get("/definitely/not/a/route")
    assert resp.status_code == 404

    metrics = (await client.get("/metrics")).text
    assert 'endpoint="__unmatched__"' in metrics
    assert 'endpoint="/definitely/not/a/route"' not in metrics
