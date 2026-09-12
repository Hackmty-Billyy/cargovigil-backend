CREATE TABLE roles (
    id   SMALLSERIAL PRIMARY KEY,
    name VARCHAR(30) NOT NULL UNIQUE
);

INSERT INTO roles (name) VALUES ('admin'), ('operations'), ('finance');
