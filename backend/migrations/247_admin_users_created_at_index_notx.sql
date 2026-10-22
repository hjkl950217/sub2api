-- Migration: 247_admin_users_created_at_index
-- The admin user list defaults to ORDER BY created_at DESC with an OFFSET
-- pagination over the whole users table. Without a matching index PostgreSQL
-- sorts every user row on each page, which is the slow path behind #7975.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_created_at_id
ON users (created_at DESC, id DESC);
