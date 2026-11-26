-- +goose Up
CREATE TABLE cities (
    id         bigserial PRIMARY KEY,
    name       text NOT NULL,
    region     text NOT NULL DEFAULT '',
    timezone   text NOT NULL,
    population integer NOT NULL DEFAULT 0,
    lat        double precision,
    lon        double precision
);
CREATE INDEX cities_name_idx ON cities (lower(name) text_pattern_ops);
CREATE INDEX cities_population_idx ON cities (population DESC);

CREATE TABLE users (
    id            bigserial PRIMARY KEY,
    email         text NOT NULL,
    password_hash text NOT NULL,
    city_id       bigint NOT NULL REFERENCES cities(id),
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_uq ON users (lower(email));

CREATE TABLE sessions (
    id           bigserial PRIMARY KEY,
    user_id      bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   bytea NOT NULL UNIQUE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE api_tokens (
    id           bigserial PRIMARY KEY,
    user_id      bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         text NOT NULL,
    token_hash   bytea NOT NULL UNIQUE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    expires_at   timestamptz,
    revoked_at   timestamptz
);
CREATE INDEX api_tokens_user_idx ON api_tokens (user_id);

CREATE TABLE password_reset_tokens (
    id         bigserial PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE password_reset_tokens, api_tokens, sessions, users, cities;
