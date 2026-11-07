-- Sales are the totals of Completed Orders, on the Bangkok day each was completed
-- (dashboard-and-reports R1, R2). Bangkok has no daylight saving, so a day is always 24
-- hours. A day with no Sales has no row: the API adds those as 0. The range is half open,
-- from the first instant of the first day to the first instant after the last.
-- name: SalesByDay :many
SELECT to_char(completed_at AT TIME ZONE 'Asia/Bangkok', 'YYYY-MM-DD')::text AS day,
    count(*)::bigint AS orders_completed,
    sum(total)::bigint AS sales
FROM orders
WHERE status = 'completed'
    AND completed_at >= sqlc.arg(from_at)::timestamptz
    AND completed_at < sqlc.arg(to_at)::timestamptz
GROUP BY 1
ORDER BY 1;

-- The Products with the highest Sales in the range, each with the quantity sold
-- (dashboard-and-reports R8), at the prices the Orders were created with (orders R3). The
-- name and SKU are as they are now. A tie in Sales goes to the larger quantity, then the
-- name (compared as ListProducts does), so the order is the same on any database.
-- name: TopSellingProducts :many
SELECT products.id, products.name, products.sku,
    sum(order_lines.quantity)::bigint AS quantity_sold,
    sum(order_lines.quantity::bigint * order_lines.price)::bigint AS sales
FROM order_lines
JOIN orders ON orders.id = order_lines.order_id
JOIN products ON products.id = order_lines.product_id
WHERE orders.status = 'completed'
    AND orders.completed_at >= sqlc.arg(from_at)::timestamptz
    AND orders.completed_at < sqlc.arg(to_at)::timestamptz
GROUP BY products.id
ORDER BY sales DESC, quantity_sold DESC, lower(products.name) COLLATE "C", products.name COLLATE "C", products.id
LIMIT sqlc.arg(row_limit);

-- The Active Products that are Out of stock or Low, the Out of stock first and then the
-- lowest stock: an Out of stock Product has the lowest stock there is
-- (dashboard-and-reports R5, products R9). Ties go to the name, as ListProducts sorts.
-- name: ListProductsToRestock :many
SELECT * FROM products
WHERE status = 'active' AND stock_state IN ('out-of-stock', 'low')
ORDER BY stock, lower(name) COLLATE "C", name COLLATE "C", id
LIMIT sqlc.arg(row_limit);
