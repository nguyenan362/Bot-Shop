-- 001_init.sql
-- Bot Shop Database Schema

-- Users
CREATE TABLE IF NOT EXISTS users (
    tele_id     BIGINT PRIMARY KEY,
    username    TEXT DEFAULT '',
    balance_usdt DECIMAL(18,8) DEFAULT 0,
    language    TEXT DEFAULT 'vi',
    join_date   TIMESTAMPTZ DEFAULT NOW(),
    is_admin    BOOLEAN DEFAULT false
);

-- Products (loại tài khoản)
CREATE TABLE IF NOT EXISTS products (
    id             SERIAL PRIMARY KEY,
    name_vi        TEXT NOT NULL,
    name_en        TEXT NOT NULL,
    price_usdt     DECIMAL(10,2) NOT NULL,
    stock          INT DEFAULT 0,
    description_vi TEXT DEFAULT '',
    description_en TEXT DEFAULT '',
    active         BOOLEAN DEFAULT true,
    created_at     TIMESTAMPTZ DEFAULT NOW()
);

-- Stock accounts (tài khoản thực tế)
CREATE TABLE IF NOT EXISTS product_accounts (
    id           SERIAL PRIMARY KEY,
    product_id   INT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    account_data TEXT NOT NULL,       -- "email:pass:info..."
    used         BOOLEAN DEFAULT false,
    order_id     BIGINT,
    created_at   TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_product_accounts_available ON product_accounts(product_id, used) WHERE used = false;

-- Orders
CREATE TABLE IF NOT EXISTS orders (
    id            BIGSERIAL PRIMARY KEY,
    user_tele_id  BIGINT NOT NULL REFERENCES users(tele_id),
    product_id    INT NOT NULL REFERENCES products(id),
    quantity      INT NOT NULL,
    total_usdt    DECIMAL(18,8) NOT NULL,
    status        TEXT DEFAULT 'pending',   -- pending, success, failed
    created_at    TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_orders_user ON orders(user_tele_id);

-- Deposits (nạp tiền qua Binance Pay)
CREATE TABLE IF NOT EXISTS deposits (
    id               BIGSERIAL PRIMARY KEY,
    user_tele_id     BIGINT NOT NULL REFERENCES users(tele_id),
    merchant_trade_no TEXT UNIQUE NOT NULL,
    amount_usdt      DECIMAL(18,8) NOT NULL,
    status           TEXT DEFAULT 'pending',  -- pending, paid, expired
    pay_url          TEXT DEFAULT '',
    created_at       TIMESTAMPTZ DEFAULT NOW(),
    paid_at          TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_deposits_trade ON deposits(merchant_trade_no);

-- Notes (lưu ý cho user)
CREATE TABLE IF NOT EXISTS notes (
    id         SERIAL PRIMARY KEY,
    content_vi TEXT DEFAULT '',
    content_en TEXT DEFAULT '',
    active     BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Binance Config (chỉ 1 record)
CREATE TABLE IF NOT EXISTS binance_config (
    id             INT PRIMARY KEY DEFAULT 1,
    merchant_id    TEXT DEFAULT '',
    api_key        TEXT DEFAULT '',
    secret_key     TEXT DEFAULT '',
    certificate_sn TEXT DEFAULT '',
    public_key     TEXT DEFAULT '',
    updated_at     TIMESTAMPTZ DEFAULT NOW()
);

-- Insert default binance_config row
INSERT INTO binance_config (id) VALUES (1) ON CONFLICT DO NOTHING;
