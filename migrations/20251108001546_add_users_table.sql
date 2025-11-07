-- +up
-- SQL statements for migration UP go here
CREATE TABLE users (
    id VARCHAR PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL
);

-- +down
-- SQL statements for migration DOWN go here
DROP TABLE users;