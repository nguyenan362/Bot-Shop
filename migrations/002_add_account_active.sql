-- 002_add_account_active.sql
-- Add active flag to product_accounts

ALTER TABLE product_accounts ADD COLUMN IF NOT EXISTS active BOOLEAN DEFAULT true;
ALTER TABLE product_accounts ADD COLUMN IF NOT EXISTS activated_at TIMESTAMPTZ;

-- Update existing rows: set active = true for any NULL values
UPDATE product_accounts SET active = true WHERE active IS NULL;

-- Rebuild available index to include active check
DROP INDEX IF EXISTS idx_product_accounts_available;
CREATE INDEX IF NOT EXISTS idx_product_accounts_available ON product_accounts(product_id, used, active) WHERE used = false AND active = true;
