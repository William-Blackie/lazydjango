#!/bin/bash

# Create compose.yaml
cat > compose.yaml << 'EOF'
services:
  postgres:
    image: postgres:16-alpine
    container_name: demo-postgres
    restart: unless-stopped
    ports:
      - "5432:5432"
    environment:
      - POSTGRES_DB=demodb
      - POSTGRES_USER=demo_user
      - POSTGRES_PASSWORD=demo_password
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD", "pg_isready", "-U", "demo_user", "-d", "demodb"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  postgres_data:
EOF

# Create .env file
cat > build/.env << 'EOF'
DB_HOST=postgres
DB_NAME=demodb
DB_PORT=5432
DB_USER=demo_user
DB_PASSWORD=demo_password
EOF

# Create manage.py
cat > manage.py << 'EOF'
#!/usr/bin/env python
import os
import sys

if __name__ == '__main__':
    os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'mysite.settings')
    try:
        from django.core.management import execute_from_command_line
    except ImportError as exc:
        raise ImportError("Couldn't import Django") from exc
    execute_from_command_line(sys.argv)
EOF

chmod +x manage.py

echo "Demo project setup complete!"
