import argparse
import asyncio
import os
import random
import string
import time
from collections.abc import Iterable

from sqlalchemy import delete, select
from sqlalchemy.dialects.postgresql import insert as pg_insert
from sqlalchemy.ext.asyncio import AsyncSession

from app.db import AsyncSessionLocal
from app.models import Item

# ---- helpers ----


def mk_random_name(prefix: str = "Item", n: int = 6, unique_suffix: bool = False) -> str:
    """
    Generates a random name.
    If unique_suffix=True, adds timestamp for uniqueness.
    """
    suffix = "".join(random.choices(string.ascii_lowercase + string.digits, k=n))
    if unique_suffix:
        # Add timestamp for guaranteed uniqueness
        timestamp_suffix = str(int(time.time() * 1000))[-6:]  # last 6 digits of timestamp
        return f"{prefix}-{suffix}-{timestamp_suffix}"
    return f"{prefix}-{suffix}"


def default_items(count: int, deterministic: bool = False) -> list[str]:
    """
    Generates a list of item names.
    If deterministic=True, uses seed(42) for stability in tests.
    If deterministic=False, generates unique names with timestamp, random component and index.
    """
    if deterministic:
        # Deterministic — for stable integration tests
        random.seed(42)
        return [mk_random_name("Item", 6, unique_suffix=False) for _ in range(count)]
    else:
        # Generate unique names with timestamp (milliseconds), random component and index
        # Use time.time_ns() for maximum uniqueness
        base_timestamp = str(int(time.time_ns() / 1_000_000))[-8:]  # last 8 digits of timestamp in ms
        # Generate unique random component for each item
        return [
            f"Item-{''.join(random.choices(string.ascii_lowercase + string.digits, k=6))}-{base_timestamp}-{''.join(random.choices(string.ascii_lowercase + string.digits, k=4))}-{i:03d}"
            for i in range(count)
        ]


# ---- seeding ----


async def seed_items(db: AsyncSession, names: Iterable[str]) -> int:
    """
    Idempotent seeding: ON CONFLICT (name) DO NOTHING.
    Returns the number of newly created records.
    """
    values = [{"name": name} for name in names]
    stmt = (
        pg_insert(Item).values(values).on_conflict_do_nothing(index_elements=[Item.name]).returning(Item.id)  # returns only new ones
    )
    res = await db.execute(stmt)
    await db.commit()
    inserted = len(res.fetchall())
    return inserted


async def reset_items(db: AsyncSession) -> int:
    res = await db.execute(delete(Item))
    await db.commit()
    # rowcount can be None depending on driver; works with asyncpg
    return res.rowcount or 0  # type: ignore[attr-defined]


async def count_items(db: AsyncSession) -> int:
    res = await db.execute(select(Item))
    return len(res.scalars().all())


# ---- CLI ----


async def main() -> None:
    parser = argparse.ArgumentParser(description="Seed PostgreSQL with test data for integration tests.")
    parser.add_argument("--count", type=int, default=int(os.getenv("SEED_COUNT", "20")), help="How many records to create (default 20 or SEED_COUNT).")
    parser.add_argument("--reset", action="store_true", help="Clear table before seeding.")
    parser.add_argument("--only-reset", action="store_true", help="Only clear and exit (without seeding).")
    parser.add_argument("--dry-run", action="store_true", help="Don't write, only show what would be created.")
    parser.add_argument("--deterministic", action="store_true", help="Use deterministic names (for tests).")
    args = parser.parse_args()

    async with AsyncSessionLocal() as db:
        if args.reset or args.only_reset:
            deleted = await reset_items(db)
            print(f"[seed] reset: deleted {deleted} rows")
            if args.only_reset:
                return

        # Use deterministic names only if --deterministic is explicitly specified
        names = default_items(args.count, deterministic=args.deterministic)

        if args.dry_run:
            print(f"[seed] dry-run: would insert {len(names)} items")
            return

        inserted = await seed_items(db, names)
        total = await count_items(db)
        print(f"[seed] inserted {inserted} new items; total now {total}")


if __name__ == "__main__":
    asyncio.run(main())
