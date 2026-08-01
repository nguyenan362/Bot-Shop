-- 007_note_products_junction.sql
-- Replace single product_id with many-to-many junction table

ALTER TABLE notes DROP COLUMN IF EXISTS product_id;

CREATE TABLE IF NOT EXISTS note_products (
    note_id    INT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    product_id INT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    PRIMARY KEY (note_id, product_id)
);
