-- +goose Up
CREATE TABLE plants (
    id         bigserial PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       text NOT NULL,
    emoji      text NOT NULL DEFAULT '🌱',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX plants_user_idx ON plants (user_id);

CREATE TABLE care_events (
    id                bigserial PRIMARY KEY,
    plant_id          bigint NOT NULL REFERENCES plants(id) ON DELETE CASCADE,
    title             text NOT NULL,
    category          text NOT NULL,
    description       text NOT NULL DEFAULT '',
    recurrence_kind   text NOT NULL CHECK (recurrence_kind IN ('rrule','manual')),
    recurrence_rule   text,
    recurrence_note   text NOT NULL DEFAULT '',
    next_due_at       date NOT NULL,
    last_notified_for date,
    channels          text[] NOT NULL DEFAULT '{}',
    active            boolean NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX care_events_plant_idx ON care_events (plant_id);
CREATE INDEX care_events_due_idx ON care_events (next_due_at) WHERE active;

CREATE TABLE plant_lookup_cache (
    query      text PRIMARY KEY,
    found      boolean NOT NULL,
    facts      jsonb,
    fetched_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE plant_lookup_cache, care_events, plants;
