from sqlalchemy import delete, select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession

from app.models import Item
from app.schemas import ItemCreate


async def list_items(db: AsyncSession) -> list[Item]:
    res = await db.execute(select(Item).order_by(Item.id))
    return list(res.scalars().all())


async def create_item(db: AsyncSession, data: ItemCreate) -> Item:
    item = Item(name=data.name)
    db.add(item)
    try:
        await db.commit()
        await db.refresh(item)
        return item
    except IntegrityError:
        await db.rollback()
        raise


async def delete_item(db: AsyncSession, item_id: int) -> bool:
    try:
        res = await db.execute(delete(Item).where(Item.id == item_id))
        await db.commit()
        # rowcount is available on Result objects from delete statements
        rowcount: int | None = res.rowcount  # type: ignore[attr-defined]
        return (rowcount or 0) > 0
    except Exception:
        await db.rollback()
        raise
