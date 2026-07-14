-- 000001_orders.up.sql
-- Initial schema for order_db: orders + order_items tables.
-- Reference: docs/system-architecture.md (Order status state machine).
--
-- Design decisions (see docs/adr if defense is needed):
--   - status: VARCHAR + CHECK instead of an ENUM type. Reason: adding/changing a
--     state does not require ALTER TYPE (Postgres ENUMs are awkward to migrate),
--     and CHECK is strict enough for data integrity.
--   - version: optimistic lock for order (low conflict). The app increments it
--     in the UPDATE; there is NO updated_at trigger, to avoid overwriting the
--     value the app sets in the optimistic-lock UPDATE
--     (UPDATE ... SET version = version + 1, updated_at = NOW()
--      WHERE id = $1 AND version = $expected).
--   - gen_random_uuid(): built in since PostgreSQL 13+, no pgcrypto extension needed.

CREATE TABLE orders (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL,
    total_amount    DECIMAL(10, 2) NOT NULL CHECK (total_amount > 0),
    status          VARCHAR(30) NOT NULL DEFAULT 'PENDING'
                        CHECK (status IN (
                            'PENDING',
                            'INVENTORY_RESERVED',
                            'PAYMENT_PROCESSING',
                            'COMPLETED',
                            'PAYMENT_FAILED',
                            'INVENTORY_INSUFFICIENT',
                            'CANCELLED'
                        )),
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    version         INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for common queries: list orders by user, filter by status.
CREATE INDEX idx_orders_user_id ON orders (user_id);
CREATE INDEX idx_orders_status ON orders (status);

CREATE TABLE order_items (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    UUID NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    product_id  UUID NOT NULL,
    quantity    INT NOT NULL CHECK (quantity > 0),
    price       DECIMAL(10, 2) NOT NULL CHECK (price >= 0)
);

-- FK lookup order_items -> orders (load items by order_id).
CREATE INDEX idx_order_items_order_id ON order_items (order_id);
