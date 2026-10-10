"""add description to items"""
from alembic import op
import sqlalchemy as sa

revision = "20260908_0002"
down_revision = "20251026_0001"
branch_labels = None
depends_on = None


def upgrade():
    op.add_column("items", sa.Column("description", sa.Text(), nullable=True))


def downgrade():
    op.drop_column("items", "description")
