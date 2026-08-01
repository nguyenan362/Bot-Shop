-- 008_support_config.sql
-- Add support contact configuration
CREATE TABLE IF NOT EXISTS support_config (
    id                INT PRIMARY KEY DEFAULT 1,
    telegram_username TEXT DEFAULT '',
    custom_message_vi TEXT DEFAULT '',
    custom_message_en TEXT DEFAULT '',
    updated_at        TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO support_config (id) VALUES (1) ON CONFLICT DO NOTHING;
