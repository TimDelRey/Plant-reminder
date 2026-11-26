-- +goose Up
ALTER TABLE care_events DROP CONSTRAINT IF EXISTS care_events_recurrence_kind_check;
UPDATE care_events SET recurrence_kind = 'once' WHERE recurrence_kind = 'manual' AND recurrence_note = 'Разово';
ALTER TABLE care_events ADD CONSTRAINT care_events_recurrence_kind_check CHECK (recurrence_kind IN ('rrule', 'manual', 'once'));
ALTER TABLE care_events ADD COLUMN completed_at timestamptz;
ALTER TABLE care_events ADD COLUMN prev_due_at date;
ALTER TABLE care_events ADD COLUMN prev_active boolean;

-- +goose Down
ALTER TABLE care_events DROP COLUMN prev_active;
ALTER TABLE care_events DROP COLUMN prev_due_at;
ALTER TABLE care_events DROP COLUMN completed_at;
ALTER TABLE care_events DROP CONSTRAINT IF EXISTS care_events_recurrence_kind_check;
UPDATE care_events SET recurrence_kind = 'manual', recurrence_note = 'Разово' WHERE recurrence_kind = 'once';
ALTER TABLE care_events ADD CONSTRAINT care_events_recurrence_kind_check CHECK (recurrence_kind IN ('rrule', 'manual'));
