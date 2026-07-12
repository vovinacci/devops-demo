from alembic import op
import sqlalchemy as sa

revision = "20251026_0001"
down_revision = None
branch_labels = None
depends_on = None

def upgrade() -> None:
    op.create_table(
        "items",
        sa.Column("id", sa.Integer, primary_key=True, autoincrement=True),
        sa.Column("name", sa.String(length=100), nullable=False, unique=True),
    )

def downgrade() -> None:
    op.drop_table("items")
