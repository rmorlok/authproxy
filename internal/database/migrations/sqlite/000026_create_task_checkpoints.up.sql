-- Progress of a long-running background sweep, so a retried or later run resumes
-- where the previous one stopped. A row exists only while a sweep is unfinished.
-- name identifies the sweep, step is the sweep-defined phase, and last_key is the
-- last key finished in that step ('' when the step has not started).
create table task_checkpoints
(
    name       text primary key,
    step       text not null,
    last_key   text not null,
    updated_at datetime    not null
);
