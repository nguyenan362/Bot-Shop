-- 006_note_after_purchase.sql
-- Add show_after_purchase and optional product_id to notes
ALTER TABLE notes ADD COLUMN IF NOT EXISTS show_after_purchase BOOLEAN DEFAULT false;
ALTER TABLE notes ADD COLUMN IF NOT EXISTS product_id INT REFERENCES products(id) ON DELETE SET NULL;
