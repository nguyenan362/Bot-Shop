-- 003_user_timezone.sql
-- Add timezone column to users table for localized broadcast scheduling.

ALTER TABLE users ADD COLUMN IF NOT EXISTS timezone TEXT DEFAULT 'Asia/Ho_Chi_Minh';
