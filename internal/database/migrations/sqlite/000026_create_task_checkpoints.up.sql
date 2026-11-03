-- Progress saved by a long-running background task, so a retried or later run
-- resumes where the previous one stopped. task_type and scope identify the work:
-- scope is '' for a task with one logical run, or separates parameterized runs of
-- the same task. payload is task-defined JSON. revision is a unique token replaced
-- by every write, which writers must present to prove they saw the latest progress.
-- Rows past expires_at count as absent and are removed by a scheduled cleanup task.
create table task_checkpoints
(
    task_type  text     not null,
    scope      text     not null,
    revision   text     not null,
    payload    text     not null,
    expires_at datetime not null,
    created_at datetime not null,
    updated_at datetime not null,
    primary key (task_type, scope)
);

create index idx_task_checkpoints_expires_at
    on task_checkpoints (expires_at);
