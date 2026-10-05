-- 010_notify_out_of_stock.sql
-- Add notify_out_of_stock toggle to products
ALTER TABLE products ADD COLUMN IF NOT EXISTS notify_out_of_stock BOOLEAN DEFAULT true;
