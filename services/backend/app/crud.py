from datetime import UTC, datetime

from sqlalchemy import delete, func, select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession

from app.events import ItemEventPayload, broadcaster
from app.models import Item
from app.schemas import ItemCreate


async def list_items(db: AsyncSession) -> list[Item]:
    res = await db.execute(select(Item).order_by(Item.id))
    return list(res.scalars().all())


async def count_items(db: AsyncSession) -> int:
    res = await db.execute(select(func.count()).select_from(Item))
    return int(res.scalar_one())


async def create_item(db: AsyncSession, data: ItemCreate) -> Item:
    item = Item(name=data.name)
    db.add(item)
    try:
        # flush assigns the primary key while still inside the transaction;
        # expire_on_commit=False keeps the attributes live after commit, so
        # no post-commit refresh roundtrip is needed
        await db.flush()
        await db.commit()
    except IntegrityError:
        await db.rollback()
        raise
    # emit-after-commit (RFC-0001 D3): the event is only published once the
    # row is durably committed, never before
    broadcaster.publish(ItemEventPayload(event_type="created", item_id=item.id, item_name=item.name, event_time=datetime.now(UTC)))
    return item


async def delete_item(db: AsyncSession, item_id: int) -> bool:
    try:
        # single atomic DELETE .. RETURNING: no get-then-delete window, so
        # concurrent deletes of the same row publish exactly one event
        res = await db.execute(delete(Item).where(Item.id == item_id).returning(Item.name))
        name = res.scalar_one_or_none()
        if name is None:
            await db.rollback()
            return False
        await db.commit()
    except Exception:
        await db.rollback()
        raise
    # ItemEvent always carries full row state, including on delete (the
    # last known state before removal -- proto3 field-presence discipline,
    # see items.proto)
    broadcaster.publish(ItemEventPayload(event_type="deleted", item_id=item_id, item_name=name, event_time=datetime.now(UTC)))
    return True
