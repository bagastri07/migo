-- +up
-- SQL statements for migration UP go here
CREATE TABLE products (
    id VARCHAR PRIMARY KEY,
    name TEXT NOT NULL,
    price NUMERIC NOT NULL
);

-- +down
-- SQL statements for migration DOWN go here
DROP TABLE products;