-- +goose Up
-- Домен «уведомления» (sender): подключённые каналы, outbox-очередь/история отправок, dev-заглушка.
CREATE TABLE user_channels (
    id                   bigserial PRIMARY KEY,
    user_id              bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel              text NOT NULL CHECK (channel IN ('max','vk','telegram')),
    external_id          text,
    status               text NOT NULL CHECK (status IN ('pending','connected','disconnected')),
    link_code            text,
    link_code_expires_at timestamptz,
    connected_at         timestamptz,
    UNIQUE (user_id, channel)
);
CREATE UNIQUE INDEX user_channels_code_uq ON user_channels (channel, link_code) WHERE link_code IS NOT NULL;

-- outbox: и очередь отправки, и история отправок
CREATE TABLE notification_jobs (
    id            bigserial PRIMARY KEY,
    care_event_id bigint NOT NULL REFERENCES care_events(id) ON DELETE CASCADE,
    user_id       bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel       text NOT NULL CHECK (channel IN ('max','vk','telegram')),
    title         text NOT NULL,   -- снимок сообщения, чтобы sender не зависел от доменной модели растений
    body          text NOT NULL,
    scheduled_at  timestamptz NOT NULL,
    status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','sent','failed')),
    attempts      integer NOT NULL DEFAULT 0,
    last_error    text,
    sent_at       timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (care_event_id, scheduled_at, channel)
);
CREATE INDEX notification_jobs_pending_idx ON notification_jobs (scheduled_at) WHERE status = 'pending';
CREATE INDEX notification_jobs_user_idx ON notification_jobs (user_id);

-- Только dev: «отправленные» заглушкой сообщения (DEV_FAKE_SENDERS)
CREATE TABLE dev_outbox (
    id          bigserial PRIMARY KEY,
    user_id     bigint,
    channel     text NOT NULL,
    external_id text NOT NULL,
    title       text NOT NULL,
    body        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE dev_outbox, notification_jobs, user_channels;
