package store

const scheduledTasksTableColumnsSQL = `(
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	task_prompt TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 0,
	running INTEGER NOT NULL DEFAULT 0,
	schedule_type TEXT NOT NULL,
	run_at TEXT NOT NULL DEFAULT '',
	cron_expressions TEXT NOT NULL DEFAULT '[]',
	timezone TEXT NOT NULL DEFAULT '',
	interval_minutes INTEGER NOT NULL DEFAULT 0,
	context_mode TEXT NOT NULL DEFAULT 'stateless',
	next_run_at TEXT NOT NULL DEFAULT '',
	last_run_at TEXT NOT NULL DEFAULT '',
	last_status TEXT NOT NULL DEFAULT '',
	last_error TEXT NOT NULL DEFAULT '',
	session_id TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
)`
