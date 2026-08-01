-- 005_show_description.sql
-- Add show_description toggle to products
ALTER TABLE products ADD COLUMN IF NOT EXISTS show_description BOOLEAN DEFAULT false;
