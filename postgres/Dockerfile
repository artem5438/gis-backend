FROM imresamu/postgis:16-3.5

# The PGDG package adds pgvector to the same PostgreSQL that already has PostGIS.
RUN apt-get update \
    && apt-get install -y --no-install-recommends postgresql-16-pgvector \
    && rm -rf /var/lib/apt/lists/*
