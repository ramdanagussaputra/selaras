-- 0002 auth down: drop in FK order (refresh_tokens references users).
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS users;
-- citext is left installed; it is harmless and may be used by later migrations.
