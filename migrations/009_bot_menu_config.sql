-- 009_bot_menu_config.sql
-- Add Telegram bot menu visibility settings.

CREATE TABLE IF NOT EXISTS bot_menu_config (
    id              INT PRIMARY KEY DEFAULT 1,
    show_notes_menu BOOLEAN DEFAULT true,
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO bot_menu_config (id) VALUES (1) ON CONFLICT DO NOTHING;
