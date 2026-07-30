PRAGMA foreign_keys = ON;

CREATE TABLE schema_version (
	version INTEGER NOT NULL
);

INSERT INTO schema_version (version) VALUES (6);

CREATE TABLE projects (
	project_id TEXT PRIMARY KEY,
	project_name TEXT,
	repository_identity TEXT NOT NULL,
	source_revision_id TEXT,
	config_path TEXT NOT NULL,
	source_object_id TEXT,
	config_sha256 TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE workflows (
	workflow_id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	workflow_name TEXT,
	repository_identity TEXT NOT NULL,
	source_revision_id TEXT,
	workflow_path TEXT NOT NULL,
	source_object_id TEXT,
	workflow_sha256 TEXT NOT NULL,
	created_at TEXT NOT NULL,

	FOREIGN KEY (project_id) REFERENCES projects(project_id)
);

CREATE TABLE workflow_instances (
	run_id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL,
	workflow_id TEXT NOT NULL,
	submission_context_json TEXT NOT NULL CHECK (json_valid(submission_context_json)),
	created_at TEXT NOT NULL,

	FOREIGN KEY (project_id) REFERENCES projects(project_id),
	FOREIGN KEY (workflow_id) REFERENCES workflows(workflow_id)
);

CREATE TABLE workflow_stages (
	run_id TEXT NOT NULL,
	stage_index INTEGER NOT NULL,
	step_id TEXT NOT NULL,
	stage_source_reference TEXT NOT NULL,
	state TEXT NOT NULL CHECK (state IN ('ready', 'running', 'completed', 'failed', 'skipped', 'blocked')),
	created_at TEXT NOT NULL,
	ready_at TEXT,
	started_at TEXT,
	completed_at TEXT,
	failed_at TEXT,
	output_json TEXT CHECK (output_json IS NULL OR json_valid(output_json)),
	output_json_sha256 TEXT,

	PRIMARY KEY (run_id, stage_index),
	FOREIGN KEY (run_id) REFERENCES workflow_instances(run_id)
);

CREATE TABLE workflow_dependency_steps (
	run_id TEXT NOT NULL,
	stage_index INTEGER NOT NULL,
	step_index INTEGER NOT NULL,
	step_id TEXT NOT NULL,
	parallel_with TEXT NOT NULL,
	created_at TEXT NOT NULL,

	PRIMARY KEY (run_id, step_index),
	UNIQUE (run_id, stage_index, step_id),
	FOREIGN KEY (run_id) REFERENCES workflow_instances(run_id)
);

CREATE TABLE workflow_dependency_work_items (
	run_id TEXT NOT NULL,
	stage_index INTEGER NOT NULL,
	step_index INTEGER NOT NULL,
	work_item_id TEXT NOT NULL,
	work_item_index INTEGER NOT NULL,
	created_at TEXT NOT NULL,

	PRIMARY KEY (run_id, work_item_id),
	UNIQUE (run_id, step_index, work_item_index),
	FOREIGN KEY (run_id, step_index) REFERENCES workflow_dependency_steps(run_id, step_index)
);

CREATE TABLE workflow_step_output_facts (
	run_id TEXT NOT NULL,
	step_index INTEGER NOT NULL,
	output_json TEXT CHECK (output_json IS NULL OR json_valid(output_json)),
	output_json_sha256 TEXT NOT NULL,
	output_json_bytes INTEGER NOT NULL,
	output_json_pruned INTEGER NOT NULL CHECK (output_json_pruned IN (0, 1)),
	output_kind TEXT NOT NULL CHECK (output_kind IN ('aggregate', 'empty_fanout', 'skipped')),
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,

	PRIMARY KEY (run_id, step_index),
	FOREIGN KEY (run_id, step_index) REFERENCES workflow_dependency_steps(run_id, step_index)
);

CREATE TABLE work_items (
	work_item_id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	stage_index INTEGER NOT NULL,
	work_item_index INTEGER NOT NULL,
	worker_payload_json TEXT NOT NULL CHECK (json_valid(worker_payload_json)),
	resolved_inputs_sha256 TEXT NOT NULL,
	created_at TEXT NOT NULL,

	UNIQUE (run_id, stage_index, work_item_index),
	FOREIGN KEY (run_id, stage_index) REFERENCES workflow_stages(run_id, stage_index)
);

CREATE TABLE workers (
	worker_id TEXT PRIMARY KEY,
	run_id TEXT,
	execution_handle TEXT,
	created_at TEXT NOT NULL,

	FOREIGN KEY (run_id) REFERENCES workflow_instances(run_id)
);

CREATE TABLE worker_sessions (
	worker_session_id TEXT PRIMARY KEY,
	worker_id TEXT NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('active', 'stopped', 'dead')),
	registered_at TEXT NOT NULL,
	last_heartbeat_at TEXT NOT NULL,
	ended_at TEXT,
	end_reason TEXT,
	execution_handle TEXT,

	UNIQUE (worker_id, worker_session_id),
	FOREIGN KEY (worker_id) REFERENCES workers(worker_id),
	CHECK (
		(status = 'active' AND ended_at IS NULL)
		OR
		(status IN ('stopped', 'dead') AND ended_at IS NOT NULL)
	)
);

CREATE TABLE work_item_attempts (
	attempt_id TEXT PRIMARY KEY,
	work_item_id TEXT NOT NULL,
	worker_id TEXT,
	worker_session_id TEXT,
	executor_type TEXT NOT NULL CHECK (executor_type IN ('worker', 'controller')),
	started_at TEXT NOT NULL,

	FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
	FOREIGN KEY (worker_id) REFERENCES workers(worker_id),
	FOREIGN KEY (worker_session_id) REFERENCES worker_sessions(worker_session_id),
	FOREIGN KEY (worker_id, worker_session_id) REFERENCES worker_sessions(worker_id, worker_session_id),
	CHECK (
		(executor_type = 'worker' AND worker_id IS NOT NULL AND worker_session_id IS NOT NULL)
		OR
		(executor_type = 'controller' AND worker_id IS NULL AND worker_session_id IS NULL)
	)
);

CREATE TABLE queued_work (
	work_item_id TEXT PRIMARY KEY,
	queued_at TEXT NOT NULL,

	FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id)
);

CREATE TABLE work_item_resource_constraints (
	work_item_id TEXT NOT NULL,
	constraint_index INTEGER NOT NULL,
	resource_key TEXT NOT NULL,
	requested_units INTEGER NOT NULL,
	operator TEXT NOT NULL CHECK (operator IN ('=', '!=', '<', '>', '<=', '>=')),
	target_units INTEGER NOT NULL,
	created_at TEXT NOT NULL,

	PRIMARY KEY (work_item_id, constraint_index),
	UNIQUE (work_item_id, resource_key),
	FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),

	CHECK (constraint_index >= 0),
	CHECK (resource_key <> ''),
	CHECK (requested_units > 0),
	CHECK (target_units >= 0)
);

CREATE TABLE running_work (
	attempt_id TEXT PRIMARY KEY,
	work_item_id TEXT NOT NULL UNIQUE,
	worker_id TEXT,
	worker_session_id TEXT,
	queued_at TEXT NOT NULL,
	started_at TEXT NOT NULL,

	FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
	FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
	FOREIGN KEY (worker_id) REFERENCES workers(worker_id),
	FOREIGN KEY (worker_session_id) REFERENCES worker_sessions(worker_session_id),
	FOREIGN KEY (worker_id, worker_session_id) REFERENCES worker_sessions(worker_id, worker_session_id)
);

CREATE TABLE abandoned_work (
	attempt_id TEXT PRIMARY KEY,
	work_item_id TEXT NOT NULL,
	worker_id TEXT NOT NULL,
	worker_session_id TEXT NOT NULL,
	queued_at TEXT NOT NULL,
	started_at TEXT NOT NULL,
	abandoned_at TEXT NOT NULL,
	reason TEXT NOT NULL,

	FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
	FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
	FOREIGN KEY (worker_id) REFERENCES workers(worker_id),
	FOREIGN KEY (worker_session_id) REFERENCES worker_sessions(worker_session_id)
);

CREATE TABLE completed_work (
	attempt_id TEXT PRIMARY KEY,
	work_item_id TEXT NOT NULL,
	skipped_parent_id TEXT,
	output_json TEXT NOT NULL CHECK (json_valid(output_json)),
	output_json_sha256 TEXT NOT NULL,
	pre_state_sha256 TEXT NOT NULL,
	post_state_sha256 TEXT NOT NULL,
	queued_at TEXT NOT NULL,
	started_at TEXT NOT NULL,
	completed_at TEXT NOT NULL,

	FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
	FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id),
	FOREIGN KEY (skipped_parent_id) REFERENCES completed_work(attempt_id)
);

CREATE TABLE failed_work (
	attempt_id TEXT PRIMARY KEY,
	work_item_id TEXT NOT NULL,
	error TEXT NOT NULL,
	queued_at TEXT NOT NULL,
	started_at TEXT NOT NULL,
	failed_at TEXT NOT NULL,

	FOREIGN KEY (attempt_id) REFERENCES work_item_attempts(attempt_id),
	FOREIGN KEY (work_item_id) REFERENCES work_items(work_item_id)
);

CREATE INDEX idx_work_items_run_stage_work_item
ON work_items(run_id, stage_index, work_item_id);

CREATE INDEX idx_workflow_dependency_steps_run_stage_step
ON workflow_dependency_steps(run_id, stage_index, step_index);

CREATE INDEX idx_workflow_dependency_work_items_run_stage_step_order
ON workflow_dependency_work_items(run_id, stage_index, step_index, work_item_index, work_item_id);

CREATE INDEX idx_queued_work_queued_at_work_item
ON queued_work(queued_at, work_item_id);

CREATE INDEX idx_running_work_started_at_attempt
ON running_work(started_at, attempt_id);

CREATE INDEX idx_worker_sessions_status_heartbeat
ON worker_sessions(status, last_heartbeat_at);

CREATE INDEX idx_worker_sessions_worker_registered
ON worker_sessions(worker_id, registered_at);

CREATE UNIQUE INDEX idx_running_work_one_per_worker_session
ON running_work(worker_session_id)
WHERE worker_session_id IS NOT NULL;

CREATE INDEX idx_abandoned_work_item_time
ON abandoned_work(work_item_id, abandoned_at, attempt_id);

CREATE INDEX idx_completed_work_item_completed_at
ON completed_work(work_item_id, completed_at, attempt_id);

CREATE INDEX idx_failed_work_item_failed_at
ON failed_work(work_item_id, failed_at, attempt_id);

CREATE INDEX idx_work_item_resource_constraints_resource_key
ON work_item_resource_constraints(resource_key);

CREATE INDEX idx_work_item_resource_constraints_work_item_id
ON work_item_resource_constraints(work_item_id);

CREATE VIEW queued_resource_constraint_checks AS
SELECT
	q.work_item_id,
	q.queued_at,
	c.constraint_index,
	c.resource_key,
	COALESCE((
		SELECT SUM(r.requested_units)
		FROM running_work rw
		JOIN work_item_resource_constraints r
			ON r.work_item_id = rw.work_item_id
		WHERE r.resource_key = c.resource_key
	), 0) AS total_units,
	c.requested_units,
	c.operator,
	c.target_units
FROM queued_work q
JOIN work_item_resource_constraints c
	ON c.work_item_id = q.work_item_id;
