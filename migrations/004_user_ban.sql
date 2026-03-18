-- 004_user_ban.sql
-- Add ban flag for users.

ALTER TABLE users ADD COLUMN IF NOT EXISTS is_banned BOOLEAN DEFAULT false;
