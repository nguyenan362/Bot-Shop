-- 002_binance_deposit_update.sql
-- Update schema for Binance deposit monitoring (replaces Binance Pay)

-- Update binance_config: add deposit address/network, keep old columns for compat
ALTER TABLE binance_config ADD COLUMN IF NOT EXISTS deposit_address TEXT DEFAULT '';
ALTER TABLE binance_config ADD COLUMN IF NOT EXISTS deposit_network TEXT DEFAULT '';

-- Update deposits table: add tx_id and network columns
ALTER TABLE deposits ADD COLUMN IF NOT EXISTS tx_id TEXT DEFAULT '';
ALTER TABLE deposits ADD COLUMN IF NOT EXISTS network TEXT DEFAULT '';

-- Make merchant_trade_no optional for new deposit records
ALTER TABLE deposits ALTER COLUMN merchant_trade_no SET DEFAULT '';

-- Drop old unique constraint on merchant_trade_no if it exists
-- (new deposits won't use merchant_trade_no)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'deposits_merchant_trade_no_key') THEN
        ALTER TABLE deposits DROP CONSTRAINT deposits_merchant_trade_no_key;
    END IF;
END $$;

-- Allow NULL for merchant_trade_no
ALTER TABLE deposits ALTER COLUMN merchant_trade_no DROP NOT NULL;

-- Unique index on tx_id (skip empty strings from old records)
CREATE UNIQUE INDEX IF NOT EXISTS idx_deposits_tx_id ON deposits(tx_id) WHERE tx_id != '';
