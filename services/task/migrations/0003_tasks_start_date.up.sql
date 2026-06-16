-- Optional start date for tasks, enabling a real start..end span in the timeline
-- (Gantt) view. Nullable; tasks without it fall back to created_at when charted.
ALTER TABLE tasks ADD COLUMN start_date TIMESTAMPTZ;
