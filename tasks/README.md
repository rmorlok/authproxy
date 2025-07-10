# Tasks

This package implements wrappers for API-exposed tasks. This is to allow for safe customer monitoring of customer
triggered work. This package allows Asynq tasks to be securely wrapped to bind them to specific auth contexts. In the
future this might evolve into a fully database-backed system that would allow listing all tasks for an actor.