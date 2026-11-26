-- +goose Up
ALTER TABLE user_channels DROP CONSTRAINT IF EXISTS user_channels_user_id_channel_key;
CREATE UNIQUE INDEX user_channels_recipient_uq ON user_channels (user_id, channel, external_id) WHERE external_id IS NOT NULL;
CREATE INDEX user_channels_user_channel_idx ON user_channels (user_id, channel);

ALTER TABLE notification_jobs ADD COLUMN recipient_id bigint REFERENCES user_channels(id) ON DELETE SET NULL;
UPDATE notification_jobs j SET recipient_id = uc.id
FROM user_channels uc
WHERE j.status = 'pending' AND uc.user_id = j.user_id AND uc.channel = j.channel AND uc.status = 'connected';
ALTER TABLE notification_jobs DROP CONSTRAINT IF EXISTS notification_jobs_care_event_id_scheduled_at_channel_key;
CREATE UNIQUE INDEX notification_jobs_dedup_uq ON notification_jobs (care_event_id, scheduled_at, channel, COALESCE(recipient_id, 0));

-- +goose Down
DROP INDEX notification_jobs_dedup_uq;
DELETE FROM notification_jobs a USING notification_jobs b
WHERE a.care_event_id = b.care_event_id AND a.scheduled_at = b.scheduled_at AND a.channel = b.channel AND a.id > b.id;
ALTER TABLE notification_jobs ADD CONSTRAINT notification_jobs_care_event_id_scheduled_at_channel_key UNIQUE (care_event_id, scheduled_at, channel);
ALTER TABLE notification_jobs DROP COLUMN recipient_id;

DROP INDEX user_channels_user_channel_idx;
DROP INDEX user_channels_recipient_uq;
DELETE FROM user_channels a USING user_channels b WHERE a.user_id = b.user_id AND a.channel = b.channel AND a.id > b.id;
ALTER TABLE user_channels ADD CONSTRAINT user_channels_user_id_channel_key UNIQUE (user_id, channel);
