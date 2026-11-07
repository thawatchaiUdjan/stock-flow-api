-- The number comes from the sequence (orders R2). Every Order starts Pending (orders R10).
-- name: InsertOrder :one
INSERT INTO orders (customer, note, total, created_by, created_at)
VALUES (sqlc.arg(customer), sqlc.arg(note), sqlc.arg(total), sqlc.arg(created_by), sqlc.arg(created_at))
RETURNING id, number;

-- name: InsertOrderLine :exec
INSERT INTO order_lines (order_id, product_id, quantity, price)
VALUES (sqlc.arg(order_id), sqlc.arg(product_id), sqlc.arg(quantity), sqlc.arg(price));

-- Holds the Order until the transaction ends, so a Complete and a Cancel at the same moment
-- run one after the other and the second finds the Order no longer Pending (orders R13).
-- name: LockOrder :one
SELECT id, status FROM orders WHERE number = $1 FOR UPDATE;

-- name: SetOrderCompleted :exec
UPDATE orders SET status = 'completed', completed_by = sqlc.arg(user_id)::bigint, completed_at = sqlc.arg(at)::timestamptz
WHERE id = sqlc.arg(id);

-- name: SetOrderCancelled :exec
UPDATE orders SET status = 'cancelled', cancelled_by = sqlc.arg(user_id)::bigint, cancelled_at = sqlc.arg(at)::timestamptz
WHERE id = sqlc.arg(id);

-- The Users' names as they are now.
-- name: ReadOrder :one
SELECT orders.id, orders.number, orders.customer, orders.note, orders.total, orders.status,
    orders.created_by, creator.name AS created_by_name, orders.created_at,
    orders.completed_by, completer.name AS completed_by_name, orders.completed_at,
    orders.cancelled_by, canceller.name AS cancelled_by_name, orders.cancelled_at
FROM orders
JOIN users creator ON creator.id = orders.created_by
LEFT JOIN users completer ON completer.id = orders.completed_by
LEFT JOIN users canceller ON canceller.id = orders.cancelled_by
WHERE orders.number = $1;

-- In the order they were given, with the Products' names and SKUs as they are now.
-- name: ListOrderLines :many
SELECT order_lines.product_id, products.name AS product_name, products.sku AS product_sku,
    order_lines.quantity, order_lines.price
FROM order_lines
JOIN products ON products.id = order_lines.product_id
WHERE order_lines.order_id = $1
ORDER BY order_lines.id;

-- Newest first (orders R15). The search is an Order's number, whole, or part of its
-- customer's name in any letter case; strpos rather than LIKE, so "%" and "_" mean
-- themselves (orders R16). A NULL filter matches everything.
-- name: ListOrders :many
SELECT orders.id, orders.number, orders.created_at, orders.customer,
    (SELECT count(*) FROM order_lines WHERE order_lines.order_id = orders.id)::integer AS line_count,
    orders.total, orders.status
FROM orders
WHERE (sqlc.narg(status)::text IS NULL OR orders.status = sqlc.narg(status))
    AND (sqlc.narg(search)::text IS NULL
        OR orders.number::text = sqlc.narg(search)
        OR strpos(lower(orders.customer), lower(sqlc.narg(search))) > 0)
ORDER BY orders.number DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountOrders :one
SELECT count(*) FROM orders
WHERE (sqlc.narg(status)::text IS NULL OR orders.status = sqlc.narg(status))
    AND (sqlc.narg(search)::text IS NULL
        OR orders.number::text = sqlc.narg(search)
        OR strpos(lower(orders.customer), lower(sqlc.narg(search))) > 0);
