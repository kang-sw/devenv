use std::path::{Path, PathBuf};
use std::time::Duration;

use rusqlite::{Connection, OpenFlags};

#[derive(Clone, Debug)]
pub(crate) struct ActivityRegistryAgentRecord {
    pub(crate) agent_key: String,
    pub(crate) public_name: String,
    pub(crate) backend: String,
    pub(crate) harness: String,
    pub(crate) tier: String,
    pub(crate) model: String,
    pub(crate) effort: String,
    pub(crate) session_id: String,
    pub(crate) status: String,
    pub(crate) created_at: String,
    pub(crate) updated_at: String,
    pub(crate) last_seen_at: String,
    pub(crate) last_call_at: String,
    pub(crate) last_output_path: String,
    state_path: String,
}

impl ActivityRegistryAgentRecord {
    pub(crate) fn payload_dir(&self, state_dir: &Path) -> Option<PathBuf> {
        safe_relative_payload_path(&self.state_path)
            .map(|relative| state_dir.join("agents").join(relative))
    }
}

#[derive(Clone, Debug)]
pub(crate) struct ActivityRegistryAgentInstanceRecord {
    pub(crate) instance_id: String,
    pub(crate) agent_key: String,
    pub(crate) cleanup_state: String,
    pub(crate) cleanup_attempted_at: String,
    pub(crate) cleanup_error: String,
    pub(crate) retention_eligible_at: String,
    pub(crate) retention_checked_at: String,
    pub(crate) retention_next_check_at: String,
    pub(crate) pinned: bool,
    agent: ActivityRegistryAgentRecord,
}

impl ActivityRegistryAgentInstanceRecord {
    pub(crate) fn payload_dir(&self, state_dir: &Path) -> Option<PathBuf> {
        self.agent.payload_dir(state_dir)
    }

    pub(crate) fn as_agent_record(&self) -> ActivityRegistryAgentRecord {
        self.agent.clone()
    }

    pub(crate) fn agent_record(&self) -> &ActivityRegistryAgentRecord {
        &self.agent
    }
}

pub(crate) fn read_activity_agent_records(
    state_dir: &Path,
) -> rusqlite::Result<Vec<ActivityRegistryAgentRecord>> {
    let db_path = state_dir.join("state.sqlite");
    if !db_path.is_file() {
        return Ok(Vec::new());
    }

    let connection = Connection::open_with_flags(
        db_path,
        OpenFlags::SQLITE_OPEN_READ_ONLY
            | OpenFlags::SQLITE_OPEN_NO_MUTEX
            | OpenFlags::SQLITE_OPEN_URI,
    )?;
    connection.busy_timeout(Duration::from_millis(50))?;
    let mut statement = connection.prepare(
        "SELECT agent_key, public_name, state_path, backend, harness, tier, model, effort, \
         session_id, status, created_at, updated_at, last_seen_at, last_call_at, last_output_path \
         FROM agent_defs ORDER BY agent_key",
    )?;
    let rows = statement.query_map([], |row| {
        let agent_key: String = row.get(0)?;
        Ok(ActivityRegistryAgentRecord {
            agent_key,
            public_name: row.get::<_, Option<String>>(1)?.unwrap_or_default(),
            state_path: row.get::<_, Option<String>>(2)?.unwrap_or_default(),
            backend: row.get::<_, Option<String>>(3)?.unwrap_or_default(),
            harness: row.get::<_, Option<String>>(4)?.unwrap_or_default(),
            tier: row.get::<_, Option<String>>(5)?.unwrap_or_default(),
            model: row.get::<_, Option<String>>(6)?.unwrap_or_default(),
            effort: row.get::<_, Option<String>>(7)?.unwrap_or_default(),
            session_id: row.get::<_, Option<String>>(8)?.unwrap_or_default(),
            status: row.get::<_, Option<String>>(9)?.unwrap_or_default(),
            created_at: row.get::<_, Option<String>>(10)?.unwrap_or_default(),
            updated_at: row.get::<_, Option<String>>(11)?.unwrap_or_default(),
            last_seen_at: row.get::<_, Option<String>>(12)?.unwrap_or_default(),
            last_call_at: row.get::<_, Option<String>>(13)?.unwrap_or_default(),
            last_output_path: row.get::<_, Option<String>>(14)?.unwrap_or_default(),
        })
    })?;

    let mut records = Vec::new();
    for row in rows {
        let record = row?;
        if !record.agent_key.is_empty() {
            records.push(record);
        }
    }
    Ok(records)
}

pub(crate) fn read_activity_agent_instance_records(
    state_dir: &Path,
) -> rusqlite::Result<Vec<ActivityRegistryAgentInstanceRecord>> {
    let db_path = state_dir.join("state.sqlite");
    if !db_path.is_file() {
        return Ok(Vec::new());
    }

    let connection = Connection::open_with_flags(
        db_path,
        OpenFlags::SQLITE_OPEN_READ_ONLY
            | OpenFlags::SQLITE_OPEN_NO_MUTEX
            | OpenFlags::SQLITE_OPEN_URI,
    )?;
    connection.busy_timeout(Duration::from_millis(50))?;
    let mut statement = connection.prepare(
        "SELECT instance_id, agent_key, public_name, state_path, backend, harness, tier, model, \
         effort, session_id, status, created_at, updated_at, last_seen_at, last_call_at, last_output_path, \
         cleanup_state, cleanup_attempted_at, cleanup_error, retention_eligible_at, \
         retention_checked_at, retention_next_check_at, pinned \
         FROM agent_instances ORDER BY updated_at DESC, instance_id",
    )?;
    let rows = statement.query_map([], |row| {
        let instance_id: String = row.get::<_, Option<String>>(0)?.unwrap_or_default();
        let agent_key: String = row.get::<_, Option<String>>(1)?.unwrap_or_default();
        Ok(ActivityRegistryAgentInstanceRecord {
            instance_id,
            agent_key: agent_key.clone(),
            agent: ActivityRegistryAgentRecord {
                agent_key,
                public_name: row.get::<_, Option<String>>(2)?.unwrap_or_default(),
                state_path: row.get::<_, Option<String>>(3)?.unwrap_or_default(),
                backend: row.get::<_, Option<String>>(4)?.unwrap_or_default(),
                harness: row.get::<_, Option<String>>(5)?.unwrap_or_default(),
                tier: row.get::<_, Option<String>>(6)?.unwrap_or_default(),
                model: row.get::<_, Option<String>>(7)?.unwrap_or_default(),
                effort: row.get::<_, Option<String>>(8)?.unwrap_or_default(),
                session_id: row.get::<_, Option<String>>(9)?.unwrap_or_default(),
                status: row.get::<_, Option<String>>(10)?.unwrap_or_default(),
                created_at: row.get::<_, Option<String>>(11)?.unwrap_or_default(),
                updated_at: row.get::<_, Option<String>>(12)?.unwrap_or_default(),
                last_seen_at: row.get::<_, Option<String>>(13)?.unwrap_or_default(),
                last_call_at: row.get::<_, Option<String>>(14)?.unwrap_or_default(),
                last_output_path: row.get::<_, Option<String>>(15)?.unwrap_or_default(),
            },
            cleanup_state: row.get::<_, Option<String>>(16)?.unwrap_or_default(),
            cleanup_attempted_at: row.get::<_, Option<String>>(17)?.unwrap_or_default(),
            cleanup_error: row.get::<_, Option<String>>(18)?.unwrap_or_default(),
            retention_eligible_at: row.get::<_, Option<String>>(19)?.unwrap_or_default(),
            retention_checked_at: row.get::<_, Option<String>>(20)?.unwrap_or_default(),
            retention_next_check_at: row.get::<_, Option<String>>(21)?.unwrap_or_default(),
            pinned: row.get::<_, Option<i64>>(22)?.unwrap_or_default() != 0,
        })
    })?;

    let mut records = Vec::new();
    for row in rows {
        let record = row?;
        if !record.instance_id.is_empty() && !record.agent_key.is_empty() {
            records.push(record);
        }
    }
    Ok(records)
}

fn safe_relative_payload_path(value: &str) -> Option<PathBuf> {
    let path = Path::new(value);
    if value.is_empty() || path.is_absolute() {
        return None;
    }
    let mut clean = PathBuf::new();
    for component in path.components() {
        match component {
            std::path::Component::Normal(part) => clean.push(part),
            _ => return None,
        }
    }
    (!clean.as_os_str().is_empty()).then_some(clean)
}
