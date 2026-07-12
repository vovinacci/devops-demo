# Homework Assignments for Students

This document contains a structured set of homework assignments of varying difficulty levels for students learning DevOps practices using the DevOps Demo
project as an example. The assignments cover backend development, frontend development, infrastructure configuration, and observability.

- **Level 1: Beginner**

  - Adding simple fields (description, created_at, is_active)
  - Basic CRUD operations and validation

- **Level 2: Intermediate**

  - Updating dependencies and Python version
  - Implementing pagination and filtering
  - UI/UX improvements

- **Level 3: Advanced**

  - Complex fields (tags, priority, due_date, attachments)
  - Extended API functionality
  - Additional metrics and dashboards

- **Level 4: Expert**
  - Workflow and item states
  - User system and authorization
  - Comments and JSON metadata
  - Extended observability

## Table of Contents

- [Level 1: Beginner](#level-1-beginner)
- [Level 2: Intermediate](#level-2-intermediate)
- [Level 3: Advanced](#level-3-advanced)
- [Level 4: Expert](#level-4-expert)
- [Additional Tasks (Optional)](#additional-tasks-optional)
- [Execution Instructions](#execution-instructions)
- [Evaluation Criteria](#evaluation-criteria)
- [Tips](#tips)

## Level 1: Beginner

Tasks at this level are designed for beginners and cover basic operations for adding simple fields to the Item model, creating migrations, and basic integration
with frontend and infrastructure components.

### 1.1. Add `description` Field to Item Model (Backend + Frontend + Infrastructure)

**Objective:** Learn to add new fields to an existing model, create migrations, and integrate changes between backend, frontend, and infrastructure components.

**Backend Tasks:**

1. **Update Model:**
    - Open `services/backend/app/models.py`
    - Add field `description: Mapped[str | None] = mapped_column(String(500), nullable=True)` to the `Item` model
    - Ensure the field has the correct type and length constraints

2. **Create Migration:**
    - Execute command to generate migration: `alembic revision --autogenerate -m "add_description_to_items"`
    - Check the generated migration file in `services/backend/alembic/versions/`
    - Ensure the migration adds column `description VARCHAR(500) NULL` to the `items` table

3. **Update Pydantic Schemas:**
    - Open `services/backend/app/schemas.py`
    - Add field `description: str | None = None` to `ItemCreate` schema
    - Add field `description: str | None` to `ItemOut` schema
    - Add validation: `Field(None, max_length=500)` to limit length

4. **Update CRUD Operations:**
    - Verify that CRUD operations in `services/backend/app/crud.py` automatically work with the new field
    - If needed, add explicit handling for the new field

5. **API-Level Validation:**
    - Ensure Pydantic automatically validates maximum length
    - Add tests for validation checking (optional)

**Frontend Tasks:**

1. **Update Creation Form:**
    - Open `services/frontend/src/App.jsx`
    - Add `<textarea>` element for entering description
    - Add state for storing description value: `const [description, setDescription] = useState('')`
    - Add change handler: `onChange={(e) => setDescription(e.target.value)}`

2. **Display Description:**
    - Update item list display to show description
    - Add display length limit (e.g., first 100 characters)
    - Add "Read more" button for full display of long descriptions

3. **Frontend Validation:**
    - Add maximum length check (500 characters)
    - Show character counter: `{description.length}/500`
    - Block form submission if description exceeds limit

4. **Update API Requests:**
    - Ensure `description` is included in POST request when creating item
    - Verify description is displayed after fetching items from API

**Infrastructure Tasks:**

1. **Automatic Migration Application:**
    - Verify that `docker-compose.yml` contains command for applying migrations on startup
    - Command should be: `python -m alembic -c /app/alembic.ini upgrade head`

2. **Prometheus Metrics:**
    - Open `services/backend/app/metrics.py`
    - Add metric for counting items with description: `items_with_description = Counter('items_with_description_total', 'Items with description')`
    - Add metric for items without description: `items_without_description = Counter('items_without_description_total', 'Items without description')`
    - Update CRUD operations to increment corresponding metrics

3. **Grafana Dashboard:**
    - Open Grafana (http://localhost:3000)
    - Find dashboard "DevOps Demo Dashboard"
    - Add new panel "Items with/without Description"
    - Use queries: `items_with_description_total` and `items_without_description_total`
    - Create pie chart or bar chart for visualization

**Expected Result:**

- User can add items with description through frontend form
- Description is displayed in item list with ability to expand full text
- Migration is automatically applied on `make up`
- Metrics are displayed in Grafana dashboard
- Validation works on both backend and frontend

**Verification:**

```shell
# Apply seed data
make seed

# Start services
make up

# Check in browser (http://localhost:8080)
# - Description field is displayed in form
# - Description is displayed in item list

# Check in Grafana (http://localhost:3000)
# - New metric is displayed on dashboard
```

---

### 1.2. Add `created_at` Field (Backend + Frontend + Infrastructure)

**Objective:** Learn to work with dates and times, add automatic value setting on record creation, and implement date sorting.

**Backend Tasks:**

1. **Update Model:**
    - Add field `created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())` to `Item` model
    - Use `func.now()` for automatic current time setting on creation
    - Use `timezone=True` for timezone-aware storage

2. **Create Migration:**
    - Create migration: `alembic revision --autogenerate -m "add_created_at_to_items"`
    - For existing records, set default value: `op.execute("UPDATE items SET created_at = NOW() WHERE created_at IS NULL")`

3. **Update Schemas:**
    - Add `created_at: datetime | None` to `ItemOut` schema
    - Do not add to `ItemCreate` (automatically set on backend)

4. **Sorting Endpoint:**
    - Update `GET /items` endpoint to support `sort` parameter
    - Add validation: `sort: str | None = Query(None, regex="^(created_at|name):(asc|desc)$")`
    - Implement sorting via SQLAlchemy: `order_by(Item.created_at.desc() if sort == "created_at:desc" else Item.created_at.asc())`

**Frontend Tasks:**

1. **Display Date:**
    - Install date library: `npm install date-fns`
    - Import functions: `import { formatDistanceToNow, format } from 'date-fns'`
    - Display date in convenient format: `formatDistanceToNow(new Date(item.created_at), { addSuffix: true })`
    - Alternatively: `format(new Date(item.created_at), 'dd.MM.yyyy HH:mm')`

2. **Date Sorting:**
    - Add "Sort by date" button in UI
    - Add state for storing sort direction: `const [sortBy, setSortBy] = useState('created_at:desc')`
    - Update API request to include parameter: `?sort=${sortBy}`
    - Toggle sort direction on click: `setSortBy(sortBy === 'created_at:desc' ? 'created_at:asc' : 'created_at:desc')`

3. **Visual Indication:**
    - Show arrow icon to indicate sort direction
    - Highlight active sort button

**Infrastructure Tasks:**

1. **Performance Index:**
    - Add index on `created_at` column in migration
    - Use: `op.create_index('ix_items_created_at', 'items', ['created_at'])`
    - This will improve query performance with sorting

2. **Prometheus Metrics:**
    - Add metric for average item age: `items_age_days = Histogram('items_age_days', 'Age of items in days', buckets=[1, 7, 30, 90, 365])`
    - Calculate age: `(datetime.now(timezone.utc) - item.created_at).days`
    - Update endpoint to calculate and record metric

3. **Grafana Dashboard:**
    - Add "Items Age Distribution" panel with histogram
    - Add "Items Created Over Time" panel with time series graph
    - Use query: `rate(items_created_total[5m])` for item creation graph

**Tip:** Use `datetime.utcnow()` or `func.now()` in SQLAlchemy for automatic creation time setting.

---

### 1.3. Add `is_active` Field with Archiving (Backend + Frontend + Infrastructure)

**Objective:** Learn to implement soft delete through archiving, add filtering, and manage record state.

**Backend Tasks:**

1. **Update Model:**
    - Add field `is_active: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)` to `Item` model
    - Default value: `True` (active)

2. **Create Migration:**
    - Create migration: `alembic revision --autogenerate -m "add_is_active_to_items"`
    - For existing records, set `is_active = True`: `op.execute("UPDATE items SET is_active = TRUE WHERE is_active IS NULL")`

3. **Archive Endpoint:**
    - Add `PATCH /items/{item_id}/archive` endpoint
    - Set `is_active = False` for archived item
    - Return 404 if item not found
    - Return 400 if item already archived

4. **Unarchive Endpoint:**
    - Add `PATCH /items/{item_id}/unarchive` endpoint
    - Set `is_active = True` for unarchived item

5. **Filtering in GET /items:**
    - Add parameter `include_archived: bool = Query(False)`
    - By default, show only active items: `filter(Item.is_active == True)`
    - If `include_archived=True`, show all items

**Frontend Tasks:**

1. **Archive Button:**
    - Add "Archive" button for each item
    - On click, call `PATCH /items/{item_id}/archive`
    - After successful archiving, update item list

2. **Archived Toggle:**
    - Add checkbox or toggle "Show archived"
    - When enabled, add parameter `include_archived=true` to API request
    - Update item list when toggle changes

3. **Visual Display:**
    - Display archived items in gray
    - Add strikethrough text for archived items
    - Show "Archived" badge next to archived items

4. **Unarchiving:**
    - Add "Unarchive" button for archived items
    - On click, call `PATCH /items/{item_id}/unarchive`

**Infrastructure Tasks:**

1. **Prometheus Metrics:**
    - Add metrics: `items_total = Counter('items_total', 'Total items', ['status'])`
    - Increment `items_total.labels(status='active')` and `items_total.labels(status='archived')`
    - Update CRUD operations to record metrics

2. **Grafana Dashboard:**
    - Add "Active vs Archived Items" panel with pie chart
    - Use queries: `items_total{status="active"}` and `items_total{status="archived"}`
    - Add stat panel with current count of active and archived items

3. **Grafana Alert:**
    - Create alert rule: `items_total{status="archived"} > 1000`
    - Configure notification channel (email, Slack, etc.)
    - Alert should trigger when archived items count exceeds 1000

---

## Level 2: Intermediate

Tasks at this level require deeper understanding of the system, including dependency updates, implementation of more complex logic, and performance
improvements.

### 2.1. Update Dependencies and Docker Images (Backend + Infrastructure)

**Objective:** Learn to update project dependencies, check version compatibility, and verify functionality after updates.

**Backend Tasks:**

1. **Check Current Versions:**
    - Open `services/backend/pyproject.toml`
    - Check current versions of all dependencies
    - Use `pip list --outdated` to check for outdated packages

2. **Update Dependencies:**
    - Check latest stable versions on PyPI
    - Update versions in `pyproject.toml` or `requirements.txt`
    - For production, use specific versions (not `latest`)
    - Example: `fastapi = "^0.115.0"` → `fastapi = "^0.120.0"`

3. **Compatibility Check:**
    - Check changelog of updated packages for breaking changes
    - Pay special attention to major versions (1.0 → 2.0)

4. **Testing After Update:**
    - Install updated dependencies: `pip install -e ".[dev]"`
    - Run tests: `make test-backend`
    - Check linting: `make lint-backend`
    - Check type checking: `make type-check`

**Infrastructure Tasks:**

1. **Update Docker Images:**
    - Check current versions of base images on Docker Hub
    - Update `FROM python:3.12-slim` if newer versions available
    - Check image sizes before and after update

2. **Rebuild Images:**
    - Execute: `make build-image`
    - Check size: `make image-size`
    - Compare sizes before and after update

3. **Update docker-compose:**
    - Check if changes needed in `docker-compose.yml`
    - Update service versions if needed (Prometheus, Grafana, etc.)

4. **Functionality Check:**
    - Start services: `make up`
    - Check health checks of all services
    - Verify all endpoints work correctly

**Tip:** Use `pip list --outdated` or check PyPI to find new versions.

**Expected Result:**

- All dependencies updated to latest stable versions
- Docker images rebuilt and working correctly
- Tests pass: `make test-docker`
- Image sizes haven't increased significantly (or even decreased)

---

### 2.2. Update Python from 3.12 to 3.13 (Backend + Infrastructure)

**Objective:** Learn to update Python version, check code and dependency compatibility with new version.

**Backend Tasks:**

1. **Update Configuration:**
    - Open `services/backend/pyproject.toml`
    - Update `requires-python = ">=3.13"`
    - Update `target-version = ["py313"]` in `.ruff.toml`
    - Update `python_version = "3.13"` in `[tool.mypy]` section in `pyproject.toml`

2. **Code Check:**
    - Verify code works on Python 3.13
    - Fix all deprecation warnings
    - Check new Python 3.13 features (if needed)

3. **Testing:**
    - Install Python 3.13 locally
    - Create new virtualenv: `python3.12 -m venv .venv`
    - Install dependencies: `pip install -e ".[dev]"`
    - Run tests: `make test-backend`

**Infrastructure Tasks:**

1. **Update Dockerfile:**
    - Open `services/backend/Dockerfile`
    - Update base image: `FROM python:3.13-slim`
    - Verify all commands work with Python 3.13

2. **Rebuild Images:**
    - Rebuild Docker images: `make build-image`
    - Check image sizes
    - Start services: `make up`

3. **Update Documentation:**
    - Update `README.md` with new Python version
    - Update `docs/prerequisites.md` with new version
    - Update `docs/local-setup.md` with instructions for Python 3.13

4. **Functionality Check:**
    - Run tests in Docker: `make test-docker`
    - Check Python version in container: `docker compose exec api python --version`
    - Should show: `Python 3.13.x`

**Verification:**

```shell
make build-image
make up
make test-docker
docker compose exec api python --version  # Should show 3.13.x
```

---

### 2.3. Add `category` Field with Filtering (Backend + Frontend + Infrastructure)

**Objective:** Learn to use enums for field value constraints, implement filtering on backend and frontend, and add metrics for categories.

**Backend Tasks:**

1. **Create Enum:**
    - Create enum for categories: `class ItemCategory(str, Enum): WORK = "work"; PERSONAL = "personal"; SHOPPING = "shopping"; OTHER = "other"`
    - Use `str, Enum` for compatibility with Pydantic and SQLAlchemy

2. **Update Model:**
    - Add field `category: Mapped[ItemCategory] = mapped_column(Enum(ItemCategory), nullable=False)` to `Item` model
    - Set default value: `default=ItemCategory.OTHER`

3. **Create Migration:**
    - Create migration: `alembic revision --autogenerate -m "add_category_to_items"`
    - For existing records, set default value

4. **Filtering Endpoint:**
    - Update `GET /items` to support parameter `category: ItemCategory | None = Query(None)`
    - Add filtering: `filter(Item.category == category)` if category specified

5. **Categories List Endpoint:**
    - Add `GET /categories` endpoint
    - Return list of all available categories: `[{"value": "work", "label": "Work"}, ...]`

**Frontend Tasks:**

1. **Category Selection:**
    - Add `<select>` element for category selection when creating item
    - Options: Work, Personal, Shopping, Other
    - Add state: `const [category, setCategory] = useState('other')`

2. **Category Filtering:**
    - Add buttons/checkboxes for filtering by categories
    - When category selected, add parameter `?category=${category}` to API request
    - Highlight active category

3. **Badge Display:**
    - Display badge with category name next to each item
    - Use different colors for different categories
    - Style via CSS classes

4. **Multiple Filtering:**
    - Add ability to select multiple categories simultaneously
    - Use array: `const [selectedCategories, setSelectedCategories] = useState([])`
    - Send requests for each category or implement backend support for multiple filtering

**Infrastructure Tasks:**

1. **Performance Index:**
    - Add index on `category` column in migration
    - Use: `op.create_index('ix_items_category', 'items', ['category'])`

2. **Prometheus Metrics:**
    - Add metric: `items_total = Counter('items_total', 'Total items', ['category'])`
    - Increment for each category: `items_total.labels(category=item.category.value).inc()`

3. **Grafana Dashboard:**
    - Add "Items by Category" panel with pie chart or bar chart
    - Use query: `items_total` grouped by `category`
    - Add stat panels for each category

4. **Loki Logging:**
    - Add logging when item category changes
    - Use structured logging: `logger.info("Category changed", extra={"item_id": item_id, "old_category": old, "new_category": new})`

**Tip:** Use `Enum` in Python and `<select>` in React for category selection.

---

### 2.4. Add Pagination (Backend + Frontend + Infrastructure)

**Objective:** Learn to implement pagination for large datasets, improve performance and user experience.

**Backend Tasks:**

1. **Pagination Parameters:**
    - Add parameters to `GET /items`: `skip: int = Query(0, ge=0)`, `limit: int = Query(10, ge=1, le=100)`
    - Validation: `skip >= 0`, `limit` from 1 to 100

2. **Pagination Implementation:**
    - Use SQLAlchemy `offset()` and `limit()`: `query.offset(skip).limit(limit)`
    - Calculate total: `total = await db.scalar(select(func.count()).select_from(Item))`

3. **Response with Metadata:**

    - Change response format:

      ```python
      {
        "items": [...],
        "total": 100,
        "skip": 0,
        "limit": 10,
        "has_next": True,
        "has_prev": False
      }
      ```

4. **Parameter Validation:**
    - Check that `skip` doesn't exceed `total`
    - Return 400 error if parameters invalid

**Frontend Tasks:**

1. **Pagination UI:**
    - Add "Previous" and "Next" page buttons
    - Show current page and total number of pages
    - Calculate: `currentPage = Math.floor(skip / limit) + 1`, `totalPages = Math.ceil(total / limit)`

2. **Items Per Page Selection:**
    - Add dropdown for selection: 10, 20, 50 items per page
    - On change, reset `skip` to 0 and update request

3. **Loading State:**
    - Show loading indicator while fetching new page
    - Use state: `const [loading, setLoading] = useState(false)`

4. **Navigation:**
    - Add buttons to go to first/last page
    - Show page numbers for direct navigation (if not too many pages)

**Infrastructure Tasks:**

1. **Prometheus Metrics:**
    - Add metric: `api_requests_total = Counter('api_requests_total', 'API requests', ['endpoint', 'method', 'has_pagination'])`
    - Increment with label `has_pagination=true` if pagination used

2. **Grafana Dashboard:**
    - Add pagination usage graph: `rate(api_requests_total{has_pagination="true"}[5m])`
    - Add stat panel with average items per page

3. **Alert:**
    - Create alert if average items per page exceeds 50
    - This may indicate performance issues

**Expected Result:**

- Endpoint `GET /items?skip=0&limit=10` returns pagination metadata
- Frontend displays pagination with navigation buttons
- Metrics track pagination usage
- Performance improved for large datasets

---

## Level 3: Advanced

Tasks at this level require deep understanding of the system, including work with complex relationships between models, files, and implementation of advanced
functionality.

### 3.1. Add `tags` Field (many-to-many) (Backend + Frontend + Infrastructure)

**Objective:** Learn to work with many-to-many relationships in SQLAlchemy, implement complex queries, and manage related data.

**Backend Tasks:**

1. **Create Tag Model:**
    - Create `Tag` model with fields `id`, `name` (unique)
    - Add `__tablename__ = "tags"` and `name: Mapped[str] = mapped_column(String(50), unique=True, nullable=False)`

2. **Association Table:**
    - Create association table:
      `item_tags = Table('item_tags', Base.metadata, Column('item_id', Integer, ForeignKey('items.id')), Column('tag_id', Integer, ForeignKey('tags.id')))`

3. **Many-to-Many Relationship:**
    - Add to `Item` model: `tags: Mapped[list[Tag]] = relationship("Tag", secondary=item_tags, back_populates="items")`
    - Add to `Tag` model: `items: Mapped[list[Item]] = relationship("Item", secondary=item_tags, back_populates="tags")`

4. **Create Migration:**
    - Create migration for `tags` and `item_tags` tables
    - Add unique index on `(item_id, tag_id)` to avoid duplicates

5. **Endpoints:**
    - `POST /items/{item_id}/tags` - add tag to item (create tag if doesn't exist)
    - `DELETE /items/{item_id}/tags/{tag_id}` - remove tag from item
    - `GET /items?tags=tag1,tag2` - filter by tags (items that have all specified tags)

**Frontend Tasks:**

1. **Tag Input:**
    - Add field for tag input (comma-separated or separate inputs)
    - Use input with ability to add multiple tags
    - Alternatively: use library like `react-tag-input`

2. **Tag Display:**
    - Display tags as badges/chips around each item
    - Style badges with different colors
    - Add ability to remove tags (if user has permissions)

3. **Tag Filtering:**
    - Add ability to click tag to filter items
    - Show only items containing selected tag
    - Highlight active tag

4. **Autocomplete:**
    - Add autocomplete when entering tags
    - Request list of existing tags from API
    - Suggest existing tags when typing

**Infrastructure Tasks:**

1. **Indexes:**
    - Add indexes on `item_tags` table for performance improvement
    - Indexes on `item_id` and `tag_id` separately
    - Composite index on `(item_id, tag_id)` for fast search

2. **Prometheus Metrics:**
    - Add metrics: `tags_total = Gauge('tags_total', 'Total number of tags')`
    - Add: `items_per_tag_avg = Histogram('items_per_tag_avg', 'Average items per tag')`

3. **Grafana Dashboard:**
    - Add "Most Popular Tags" panel with top 10 tags
    - Use query to count items per tag
    - Create bar chart or word cloud

4. **Loki Logging:**
    - Add logging when new tag created
    - Log tag changes for items

**Tip:** Use `Table()` for association table in SQLAlchemy and `relationship()` with `secondary` parameter.

---

### 3.2. Add `priority` Field with Validation (Backend + Frontend + Infrastructure)

**Objective:** Learn to use numeric fields with constraints, implement validation at different levels, and visualize numeric values.

**Backend Tasks:**

1. **Update Model:**
    - Add field `priority: Mapped[int] = mapped_column(Integer, nullable=False, default=3)`
    - Default value: 3 (medium priority)

2. **Pydantic Validation:**
    - Add validation in schema: `priority: int = Field(ge=1, le=5, description="Priority from 1 (lowest) to 5 (highest)")`
    - Pydantic will automatically check value is in range 1-5

3. **Create Migration:**
    - Create migration: `alembic revision --autogenerate -m "add_priority_to_items"`
    - Add check constraint: `op.create_check_constraint('ck_items_priority_range', 'items', 'priority >= 1 AND priority <= 5')`

4. **Priority Sorting:**
    - Add sorting by priority (highest first): `order_by(Item.priority.desc())`
    - Combine with other sorting (e.g., by date)

5. **Filtering Endpoint:**
    - Add parameter `priority_min: int | None = Query(None, ge=1, le=5)`
    - Filter: `filter(Item.priority >= priority_min)` if specified

**Frontend Tasks:**

1. **Visual Display:**
    - Display priority as stars (1-5 stars)
    - Alternatively: numeric indicator with color coding
    - Colors: red (5), orange (4), yellow (3), green (2), gray (1)

2. **Change Priority:**
    - Add dropdown or slider for changing priority
    - Dropdown: `<select>` with options 1-5
    - Slider: use `<input type="range" min="1" max="5">`

3. **High Priority Display:**
    - Display items with priority 4-5 at top of list
    - Highlight high priority with brighter color
    - Add "!" icon for highest priority

4. **Filtering:**
    - Add filter "Show only high priority"
    - Use parameter `priority_min=4` in API request

**Infrastructure Tasks:**

1. **Prometheus Metrics:**
    - Add metric: `items_total = Counter('items_total', 'Total items', ['priority'])`
    - Increment for each priority level: `items_total.labels(priority=str(item.priority)).inc()`

2. **Grafana Dashboard:**
    - Add "Items by Priority" panel with bar chart
    - Display distribution of items by priority
    - Add stat panels for each priority level

3. **Alert:**
    - Create alert if count of items with priority 5 exceeds 20
    - This may indicate system overload with high-priority tasks

---

### 3.3. Add `due_date` Field (Deadline) (Backend + Frontend + Infrastructure)

**Objective:** Learn to work with dates and times for deadlines, implement date validation, and track overdue tasks.

**Backend Tasks:**

1. **Update Model:**
    - Add field `due_date: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)` to `Item` model
    - Use `timezone=True` for correct timezone handling

2. **Validation:**
    - Add validation in Pydantic: `due_date: datetime | None = Field(None, description="Due date cannot be in the past")`
    - Create custom validator: `@field_validator('due_date')` to check date is not in the past

3. **Create Migration:**
    - Create migration: `alembic revision --autogenerate -m "add_due_date_to_items"`
    - Add index on `due_date` for fast search of overdue items

4. **Overdue Endpoint:**
    - Add `GET /items/overdue` endpoint
    - Filter: `filter(Item.due_date < datetime.now(timezone.utc))`
    - Return only items with `due_date` and `is_active=True`

5. **Filtering Endpoint:**
    - Add parameter `due_before: datetime | None = Query(None)`
    - Filter: `filter(Item.due_date < due_before)` if specified

**Frontend Tasks:**

1. **Date Picker:**
    - Install library: `npm install react-datepicker` or use native `<input type="date">`
    - Add date picker for selecting deadline
    - Validate date is not in the past

2. **Display Deadline:**
    - Display deadline in convenient format: "In 3 days" or "Overdue by 2 days"
    - Use `date-fns`: `formatDistanceToNow(new Date(item.due_date), { addSuffix: true })`

3. **Visual Indication:**
    - Highlight overdue items in red
    - Add "!" icon for overdue items
    - Show progress bar with proximity to deadline

4. **Overdue Filter:**
    - Add checkbox "Show only overdue"
    - When enabled, call `GET /items/overdue`
    - Update list when filter changes

**Infrastructure Tasks:**

1. **Index:**
    - Add index on `due_date` column for fast search of overdue items
    - Use: `op.create_index('ix_items_due_date', 'items', ['due_date'])`

2. **Prometheus Metrics:**
    - Add metrics: `items_overdue_total = Gauge('items_overdue_total', 'Number of overdue items')`
    - Add: `items_due_soon_total = Gauge('items_due_soon_total', 'Number of items due within 7 days')`
    - Update metrics on each request or via scheduled task

3. **Grafana Dashboard:**
    - Add "Overdue Items" panel with stat panel
    - Add "Items Due Soon" panel (deadline within < 7 days)
    - Create time series graph with trend of overdue items

4. **Alert:**
    - Create alert if count of overdue items exceeds 10
    - Configure notification for critical situation alerts

---

### 3.4. Add File Upload (Backend + Frontend + Infrastructure)

**Objective:** Learn to implement file upload and storage, handle multipart/form-data requests, and manage file storage.

**Backend Tasks:**

1. **Create Attachment Model:**
    - Create `Attachment` model with fields: `id`, `item_id` (foreign key), `filename`, `file_path`, `file_size`, `uploaded_at`
    - Add relationship to `Item`: `attachments: Mapped[list[Attachment]] = relationship("Attachment", back_populates="item")`

2. **Create Migration:**
    - Create migration for `attachments` table
    - Add foreign key constraint on `item_id`

3. **Endpoints:**
    - `POST /items/{item_id}/attachments` - upload file (multipart/form-data)
    - `GET /items/{item_id}/attachments` - list files for item
    - `GET /items/{item_id}/attachments/{attachment_id}` - download file
    - `DELETE /items/{item_id}/attachments/{attachment_id}` - delete file

4. **File Validation:**
    - Check file size (maximum 10MB)
    - Check file type (allow only certain types, e.g., images, PDF)
    - Generate unique filename to avoid conflicts

5. **File Storage:**
    - Store files in `/app/data/uploads/` directory
    - Create subdirectories by item_id for organization
    - Store metadata in database

**Frontend Tasks:**

1. **File Upload:**
    - Add `<input type="file">` for file selection
    - Support drag-and-drop via `onDrop` handler
    - Show preview for images before upload

2. **File Display:**
    - Show file list under each item
    - Display file type icon, name, size, and upload date
    - Add button to download file

3. **Progress Bar:**
    - Show progress bar during upload
    - Use `XMLHttpRequest` or `fetch` with `onUploadProgress`
    - Update progress bar during upload

4. **File Deletion:**
    - Add delete button for each file
    - Confirm deletion via confirm dialog
    - Update list after deletion

**Infrastructure Tasks:**

1. **Volume for Files:**
    - Configure volume in `docker-compose.yml`: `volumes: - ./data/uploads:/app/data/uploads`
    - Ensure directory is created automatically
    - Configure access permissions

2. **Prometheus Metrics:**
    - Add metrics: `attachments_total = Counter('attachments_total', 'Total number of attachments')`
    - Add: `attachments_size_bytes_total = Counter('attachments_size_bytes_total', 'Total size of attachments in bytes')`

3. **Grafana Dashboard:**
    - Add "Storage Usage" panel with total file size
    - Display graph of storage usage growth
    - Add stat panel with file count

4. **Alert:**
    - Create alert if total file size exceeds 1GB
    - Configure automatic cleanup of old files (optional)

5. **Backup:**
    - Configure backup for file directory
    - Use cron job or scheduled task for regular backup

**Tip:** Use `UploadFile` from FastAPI and store files in `/app/data/uploads/` directory.

---

### 3.5. Update Dependencies and Fix Breaking Changes (Backend + Infrastructure)

**Objective:** Learn to update major dependency versions, fix breaking changes, and keep project up to date.

**Backend Tasks:**

1. **Update Dependencies:**
    - Update all dependencies to latest major versions
    - Check changelog of each library for breaking changes
    - Example updates:
        - FastAPI 0.115 → 0.120+
        - SQLAlchemy 2.0.36 → 2.0.40+
        - Pydantic 2.9.2 → 2.10+
        - Uvicorn 0.30.0 → 0.32+

2. **Fix Breaking Changes:**
    - Fix API changes (deprecated functions, parameter changes)
    - Update code according to new best practices
    - Fix type hint changes

3. **Testing:**
    - Run all tests: `make test-backend`
    - Fix tests that fail due to breaking changes
    - Check linting: `make lint-backend`
    - Check type checking: `make type-check`

4. **Update Documentation:**
    - Update code examples in documentation
    - Update versions in README.md

**Infrastructure Tasks:**

1. **Update Docker Images:**
    - Update base images if needed
    - Rebuild images: `make build-image`
    - Check image sizes

2. **Compatibility Check:**
    - Check compatibility with other services (Prometheus, Grafana, Loki)
    - Update service versions in docker-compose.yml if needed

3. **CI/CD Pipeline:**
    - Update CI/CD pipeline for new versions
    - Verify all tests pass in CI

---

## Level 4: Expert

Tasks at this level require expert-level knowledge and include complex architecture, real-time functionality, and migrations to new programming language
versions.

### 4.1. Add Workflow with Statuses (Backend + Frontend + Infrastructure)

**Objective:** Learn to implement state machine for state management, validate state transitions, and visualize workflow.

**Backend Tasks:**

1. **Create Status Enum:**
    - Create enum: `class ItemStatus(str, Enum): TODO = "todo"; IN_PROGRESS = "in_progress"; REVIEW = "review"; DONE = "done"; CANCELLED = "cancelled"`

2. **Update Model:**
    - Add field `status: Mapped[ItemStatus] = mapped_column(Enum(ItemStatus), default=ItemStatus.TODO, nullable=False)`

3. **Transition Validation:**

    - Create mapping of allowed transitions:

      ```python
      ALLOWED_TRANSITIONS = {
          ItemStatus.TODO: [ItemStatus.IN_PROGRESS, ItemStatus.CANCELLED],
          ItemStatus.IN_PROGRESS: [ItemStatus.REVIEW, ItemStatus.CANCELLED],
          ItemStatus.REVIEW: [ItemStatus.DONE, ItemStatus.IN_PROGRESS],
          ItemStatus.DONE: [],
          ItemStatus.CANCELLED: []
      }
      ```

    - Validate transition before changing status

4. **Endpoints:**
    - `PATCH /items/{item_id}/status` - change status with transition validation
    - `GET /items?status=todo` - filter by status
    - `GET /items/stats` - statistics by status

**Frontend Tasks:**

1. **Kanban Board:**
    - Display items grouped by status in columns
    - Use drag-and-drop for status change
    - Library: `react-beautiful-dnd` or `@dnd-kit/core`

2. **Color Coding:**
    - Different colors for different statuses
    - Visual indication of current status

3. **Statistics:**
    - Show count of items in each status
    - Display progress bar with completion progress

**Infrastructure Tasks:**

1. **Metrics:**
    - `items_total{status="todo"}` for each status
    - `status_transitions_total{from="todo",to="in_progress"}` for workflow tracking

2. **Grafana Dashboard:**
    - Kanban-like panel "Items by Status"
    - Graph of transitions between statuses

3. **Alert:**
    - Alert if items stuck in "in_progress" status for more than 7 days

**Tip:** Use `pydantic` for transition validation or `transitions` library.

---

### 4.2. Add Users and Assignments (Backend + Frontend + Infrastructure)

**Objective:** Learn to implement user system, foreign keys between models, and assignment management.

**Backend Tasks:**

1. **User Model:**
    - Create `User` model with fields: `id`, `username`, `email`, `created_at`
    - Add uniqueness on `username` and `email`

2. **Update Item:**
    - Add `assignee_id: Mapped[int | None] = mapped_column(ForeignKey("users.id"), nullable=True)`
    - Add relationship: `assignee: Mapped[User | None] = relationship("User")`

3. **Endpoints:**
    - `GET /users` - list users
    - `POST /users` - create user
    - `PATCH /items/{item_id}/assign` - assign item to user
    - `GET /items?assignee_id=1` - filter by assigned user

**Frontend Tasks:**

1. **User Selection:**
    - Dropdown for user selection when creating/editing item
    - Display avatar/initials of assigned user

2. **Filtering:**
    - Filter by assigned user
    - "My Profile" page with list of assigned items

**Infrastructure Tasks:**

1. **Metrics:**
    - `items_total{assignee="user1"}` for each user
    - Alert if one user has more than 50 assigned items

---

### 4.3. Add Comments with Real-Time Updates (Backend + Frontend + Infrastructure)

**Objective:** Learn to implement real-time functionality via WebSockets, manage comments, and synchronize state between clients.

**Backend Tasks:**

1. **Comment Model:**
    - Create `Comment` model with fields: `id`, `item_id`, `author_id`, `content`, `created_at`, `updated_at`

2. **Endpoints:**
    - `GET /items/{item_id}/comments` - list comments
    - `POST /items/{item_id}/comments` - create comment
    - `PATCH /items/{item_id}/comments/{comment_id}` - edit
    - `DELETE /items/{item_id}/comments/{comment_id}` - delete

3. **WebSocket:**
    - Add WebSocket endpoint for real-time comment updates
    - Use FastAPI WebSocket support
    - Broadcast new comments to all connected clients

**Frontend Tasks:**

1. **Comments Section:**
    - Form for adding comments
    - Display author and creation time
    - Ability to edit and delete own comments

2. **Real-Time Updates:**
    - Connect to WebSocket
    - Automatic updates on new comments

**Infrastructure Tasks:**

1. **Metrics:**
    - `comments_total`, `comments_per_item_avg`
    - WebSocket connection monitoring

---

### 4.4. Add JSON Metadata with Search (Backend + Frontend + Infrastructure)

**Objective:** Learn to use JSONB types in PostgreSQL, implement search on JSON fields, and manage structured metadata.

**Backend Tasks:**

1. **Update Model:**
    - Add field `metadata: Mapped[dict | None] = mapped_column(JSON, nullable=True)` (PostgreSQL JSONB)

2. **Search:**
    - Endpoint for search: `GET /items?metadata.color=blue`
    - Full-text search: `GET /items/search?q=metadata.location:office`

3. **Validation:**
    - Pydantic schema for metadata
    - Structure checking

**Frontend Tasks:**

1. **Metadata UI:**
    - Key-value pairs for adding metadata
    - Search by metadata
    - Display metadata

**Infrastructure Tasks:**

1. **Indexes:**
    - GIN index on JSONB column
    - Metrics for metadata usage

**Tip:** Use `JSON` type in SQLAlchemy and `dict` in Pydantic.

---

## Additional Tasks (Optional)

### A. Testing (Backend + Frontend)

- Write unit tests for new fields and validation
- Write integration tests for new endpoints
- Add frontend tests (React Testing Library)
- Configure code coverage reports

### B. Documentation (Backend + Frontend)

- Update API documentation (Swagger/OpenAPI)
- Add usage examples for new fields
- Update README.md with description of new features
- Add JSDoc comments for React components

### C. DevOps (Infrastructure)

- Add migrations to CI/CD pipeline
- Configure automatic testing after migrations
- Add health check for new endpoints
- Configure automatic database backup
- Add monitoring for new metrics

### D. UI/UX (Frontend)

- Improve form design for new fields
- Add frontend validation (React Hook Form)
- Add loading states and error handling
- Add animations and transitions
- Make application responsive for mobile devices

---

## Execution Instructions

### General Process for Adding a Field

**1. Backend:**

```shell
# 1. Update model (models.py)
# 2. Update schemas (schemas.py)
# 3. Update CRUD (crud.py)
# 4. Update endpoints (main.py)
# 5. Create migration
cd services/backend
alembic revision --autogenerate -m "add_field_name"
# 6. Verify migration
alembic upgrade head
```

**2. Frontend:**

```shell
# 1. Update components (App.jsx)
# 2. Add fields to forms
# 3. Update data display
# 4. Verify in browser
cd services/frontend
npm run dev
```

**3. Infrastructure:**

```shell
# 1. Check docker-compose.yaml
# 2. Add Prometheus metrics (metrics.py)
# 3. Update Grafana dashboard
# 4. Restart services
make down
make up
```

**4. Testing:**

```shell
make test-docker  # Backend tests
make lint         # Code check
make type-check   # Type checking
```

---

## Evaluation Criteria

### Backend

- **Functionality:** Does the new field/endpoint work correctly?
- **Code:** Are best practices and code style followed?
- **Tests:** Are tests added for new functionality?
- **Migrations:** Are migrations created and applied correctly?
- **Validation:** Is validation added at API level?

### Frontend

- **UI/UX:** Is the interface user-friendly?
- **Validation:** Is frontend validation added?
- **Error Handling:** Are errors handled correctly?
- **Responsive:** Does it work on mobile devices?

### Infrastructure

- **Docker:** Are Docker images and volumes configured correctly?
- **Metrics:** Are Prometheus metrics added?
- **Grafana:** Are dashboards updated?
- **Logging:** Is logging added to Loki?
- **Alerts:** Are alerts configured (if needed)?

---

## Tips

- Always verify migrations on test database before production
- Use type hints for better IDE support
- Follow DRY principle (Don't Repeat Yourself)
- Write comments for complex code parts
- Test not only "happy path", but also edge cases
- Check Docker image sizes after changes
- Monitor metrics after adding new functionality
- Document changes in Infrastructure (docker-compose, volumes, etc.)
- Use Git for versioning changes
- Create separate branches for each task
- Write descriptive commit messages
- Always run tests before commit

---

**Good luck with completing the assignments!**
