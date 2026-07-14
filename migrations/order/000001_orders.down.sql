-- 000001_orders.down.sql
-- Rollback of 000001_orders.up.sql.
-- DROP order: order_items first (it has the FK to orders), then orders.
-- DROP TABLE drops that table's indexes automatically, so no separate DROP INDEX.

DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
