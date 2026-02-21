#!/bin/bash
# Demo script showing snapshot and data viewer functionality

set -e

echo "=== Lazy Django - Docker Integration Demo ==="
echo

# Check prerequisites
if ! command -v docker &> /dev/null; then
    echo "❌ Docker not found. Please install Docker first."
    exit 1
fi

echo "✅ Docker found"

# Check if compose.yaml exists
if [ ! -f "compose.yaml" ]; then
    echo "❌ compose.yaml not found. Run this script from demo-project directory."
    exit 1
fi

echo "✅ compose.yaml found"
echo

# Start PostgreSQL
echo "🚀 Starting PostgreSQL container..."
docker compose up -d postgres
sleep 3

# Wait for postgres to be ready
echo "⏳ Waiting for PostgreSQL to be ready..."
for i in {1..30}; do
    if docker compose exec -T postgres pg_isready -U demo_user &> /dev/null; then
        echo "✅ PostgreSQL is ready"
        break
    fi
    echo -n "."
    sleep 1
done
echo

# Create settings.py if needed
if [ ! -f "mysite/settings.py" ]; then
    echo "📝 Creating Django settings..."
    cat > mysite/settings.py << 'EOF'
import os

DEBUG = True
ALLOWED_HOSTS = ['*']

INSTALLED_APPS = [
    'django.contrib.contenttypes',
    'django.contrib.auth',
    'blog',
    'shop',
]

DATABASES = {
    'default': {
        'ENGINE': 'django.db.backends.postgresql',
        'NAME': os.getenv('POSTGRES_DB', 'demodb'),
        'USER': os.getenv('POSTGRES_USER', 'demo_user'),
        'PASSWORD': os.getenv('POSTGRES_PASSWORD', 'demo_password'),
        'HOST': 'localhost',
        'PORT': '5432',
    }
}

SECRET_KEY = 'demo-secret-key-not-for-production'
EOF
fi

echo "✅ Configuration ready"
echo

echo "📊 Database Type Detection Demo"
echo "================================"
echo
echo "The snapshot system automatically detects your database type:"
echo "  PostgreSQL: Uses pg_dump/psql"
echo "  MySQL: Uses mysqldump/mysql"
echo "  SQLite: Direct file copy"
echo "  Others: Django dumpdata/loaddata"
echo
echo "Your demo project uses: PostgreSQL (detected from compose.yaml)"
echo

echo "🔄 Snapshot System Features"
echo "==========================="
echo
echo "1. CREATE SNAPSHOT"
echo "   - Captures full database state"
echo "   - Stores git branch and commit"
echo "   - Records applied migrations"
echo "   - Works with Docker or local DB"
echo
echo "2. LIST SNAPSHOTS"
echo "   - Shows all saved states"
echo "   - Displays metadata (branch, time, migrations)"
echo
echo "3. RESTORE SNAPSHOT"
echo "   - Restores database to exact state"
echo "   - Auto-syncs migrations"
echo "   - Safe rollback on git branch switch"
echo
echo "4. GIT INTEGRATION"
echo "   - Auto-detects branch changes"
echo "   - Suggests relevant snapshots"
echo "   - Prevents data loss when switching branches"
echo

echo "📝 Data Viewer Features"
echo "======================"
echo
echo "1. QUERY MODELS"
echo "   - Browse all records with pagination"
echo "   - Filter by any field"
echo "   - Search across text fields"
echo "   - Works with any Django backend"
echo
echo "2. CRUD OPERATIONS"
echo "   - Create new records"
echo "   - Update existing records"
echo "   - Delete records"
echo "   - View relationships"
echo
echo "3. DATABASE AGNOSTIC"
echo "   - Uses Django ORM (works with ALL backends)"
echo "   - Executes in Docker containers"
echo "   - Type-safe field validation"
echo "   - Handles related objects"
echo

echo "🐳 Docker Integration"
echo "===================="
echo
echo "Container Detection:"
docker compose ps postgres | grep postgres || echo "  No containers running"
echo
echo "Database Connection:"
if docker compose exec -T postgres psql -U demo_user -d demodb -c "SELECT version();" &> /dev/null; then
    echo "✅ PostgreSQL is accessible"
    VERSION=$(docker compose exec -T postgres psql -U demo_user -d demodb -tc "SELECT version();" | tr -d ' \n' | cut -c1-60)
    echo "  📦 Version: $VERSION..."
else
    echo "  ⚠️  Could not connect (container may not be fully ready)"
fi
echo

echo "🎯 Usage Examples"
echo "================"
echo
echo "# Create a snapshot before making changes"
echo "snapshot := manager.CreateSnapshot(\"before-feature-x\")"
echo
echo "# Switch git branches safely"
echo "git checkout feature-branch"
echo "# lazy-django detects branch change and offers to restore snapshot"
echo
echo "# Query model data"
echo "records := viewer.QueryModel(\"blog\", \"Post\", filters, page, pageSize)"
echo
echo "# Edit a record"
echo "viewer.UpdateRecord(\"blog\", \"Post\", pk, updatedFields)"
echo
echo "# Search records"
echo "results := viewer.SearchRecords(\"blog\", \"Post\", \"Django\", 1, 50)"
echo

echo "✨ Key Benefits"
echo "=============="
echo "  ✅ Database-agnostic (PostgreSQL, MySQL, SQLite, Oracle, etc.)"
echo "  ✅ Docker-aware (auto-detects and uses containers)"
echo "  ✅ Git-integrated (tracks branches, commits)"
echo "  ✅ Safe rollbacks (snapshot before migrations)"
echo "  ✅ Full CRUD (create, read, update, delete records)"
echo "  ✅ Production-ready (comprehensive error handling)"
echo

echo "🧪 Test Results"
echo "==============="
cd ..
go test ./pkg/django/... -short -run="TestSnapshot|TestDatabase" -v 2>&1 | grep -E "PASS|FAIL|ok|--" | head -20
echo

echo "📚 Next Steps"
echo "============"
echo "1. Run: ./build.sh to compile lazy-django"
echo "2. Navigate to demo-project"
echo "3. Run: ../lazy-django to launch the TUI"
echo "4. Use new keybindings:"
echo "   - 'S': Create database snapshot"
echo "   - 'R': Restore from snapshot"
echo "   - 'L': List all snapshots"
echo "   - 'v': View model data"
echo "   - 'e': Edit record"
echo "   - 'd': Delete record"
echo "   - 'n': Create new record"
echo

echo "✅ Demo complete!"
echo
echo "Note: Snapshot files are stored in .lazy-django/snapshots/"
echo "These files are portable and can be shared between team members."
