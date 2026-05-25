use std::path::Path;
use std::time::Duration;

use rusqlite::{Connection, OpenFlags, OptionalExtension, Row};
use sha2::{Digest, Sha256};

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct AgentRegistrySnapshot {
    pub current_roles: Vec<AgentRegistryRecord>,
    pub historical_instances: Vec<AgentRegistryRecord>,
    pub degraded: bool,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AgentRegistryRecord {
    pub agent_key: String,
    pub instance_id: Option<String>,
    pub public_name: String,
    pub state_path: String,
    pub backend: String,
    pub harness: String,
    pub tier: String,
    pub model: String,
    pub effort: String,
    pub session_id: String,
    pub status: String,
    pub created_at: String,
    pub updated_at: String,
    pub last_seen_at: String,
    pub last_call_at: String,
    pub last_output_path: String,
    pub cleanup_state: String,
    pub cleanup_error: String,
}

impl AgentRegistryRecord {
    pub fn role_name(&self) -> String {
        if self.public_name.is_empty() {
            self.agent_key.clone()
        } else {
            self.public_name.clone()
        }
    }

    pub fn recency_key(&self) -> &str {
        [
            self.updated_at.as_str(),
            self.last_seen_at.as_str(),
            self.last_call_at.as_str(),
            self.created_at.as_str(),
        ]
        .into_iter()
        .find(|value| !value.is_empty())
        .unwrap_or("")
    }
}

pub fn read_agent_registry(db_path: &Path, recent_limit: Option<usize>) -> AgentRegistrySnapshot {
    if !db_path.is_file() {
        return AgentRegistrySnapshot::default();
    }

    let Ok(connection) = Connection::open_with_flags(
        db_path,
        OpenFlags::SQLITE_OPEN_READ_ONLY | OpenFlags::SQLITE_OPEN_NO_MUTEX,
    ) else {
        return AgentRegistrySnapshot {
            degraded: true,
            ..AgentRegistrySnapshot::default()
        };
    };
    let _ = connection.busy_timeout(Duration::from_millis(80));

    let mut current_roles = match query_current_roles(&connection) {
        Ok(records) => records,
        Err(_) => {
            return AgentRegistrySnapshot {
                degraded: true,
                ..AgentRegistrySnapshot::default()
            };
        }
    };
    current_roles.sort_by(registry_record_ordering);
    if let Some(limit) = recent_limit {
        current_roles.truncate(limit);
    }

    let mut historical_instances = match query_historical_instances(&connection) {
        Ok(records) => records,
        Err(_) => {
            return AgentRegistrySnapshot {
                current_roles,
                degraded: true,
                historical_instances: Vec::new(),
            };
        }
    };
    historical_instances.sort_by(registry_record_ordering);

    AgentRegistrySnapshot {
        current_roles,
        historical_instances,
        degraded: false,
    }
}

fn registry_record_ordering(
    left: &AgentRegistryRecord,
    right: &AgentRegistryRecord,
) -> std::cmp::Ordering {
    right
        .recency_key()
        .cmp(left.recency_key())
        .then_with(|| left.agent_key.cmp(&right.agent_key))
        .then_with(|| left.state_path.cmp(&right.state_path))
        .then_with(|| left.instance_id.cmp(&right.instance_id))
}

fn query_current_roles(connection: &Connection) -> rusqlite::Result<Vec<AgentRegistryRecord>> {
    let mut statement = connection.prepare(
        r#"
        SELECT
            d.agent_key,
            COALESCE(i.instance_id, ''),
            d.public_name,
            d.state_path,
            d.backend,
            d.harness,
            d.tier,
            d.model,
            d.effort,
            d.session_id,
            d.status,
            d.created_at,
            d.updated_at,
            d.last_seen_at,
            d.last_call_at,
            d.last_output_path,
            'current',
            ''
        FROM agent_defs d
        LEFT JOIN agent_instances i
            ON i.agent_key = d.agent_key
            AND i.state_path = d.state_path
        "#,
    )?;
    let rows = statement.query_map([], current_role_from_row)?;
    rows.collect()
}

fn query_historical_instances(
    connection: &Connection,
) -> rusqlite::Result<Vec<AgentRegistryRecord>> {
    let mut statement = connection.prepare(
        r#"
        SELECT
            instance_id,
            agent_key,
            public_name,
            state_path,
            backend,
            harness,
            tier,
            model,
            effort,
            session_id,
            status,
            created_at,
            updated_at,
            last_seen_at,
            last_call_at,
            last_output_path,
            cleanup_state,
            cleanup_error
        FROM agent_instances
        WHERE cleanup_state != 'current'
          AND cleanup_state != 'cleanup_deleted'
        "#,
    )?;
    let rows = statement.query_map([], historical_instance_from_row)?;
    rows.collect()
}

pub fn lookup_activity_record(
    db_path: &Path,
    activity_id: &str,
    current_prefix: &str,
    historical_prefix: &str,
) -> Option<AgentRegistryRecord> {
    let connection = Connection::open_with_flags(
        db_path,
        OpenFlags::SQLITE_OPEN_READ_ONLY | OpenFlags::SQLITE_OPEN_NO_MUTEX,
    )
    .ok()?;
    let _ = connection.busy_timeout(Duration::from_millis(80));

    if let Some(agent_key) = activity_id.strip_prefix(current_prefix) {
        if agent_key.is_empty() || agent_key.contains('/') || agent_key.contains('\\') {
            return None;
        }
        return connection
            .query_row(
                r#"
                SELECT
                    d.agent_key,
                    COALESCE(i.instance_id, ''),
                    d.public_name,
                    d.state_path,
                    d.backend,
                    d.harness,
                    d.tier,
                    d.model,
                    d.effort,
                    d.session_id,
                    d.status,
                    d.created_at,
                    d.updated_at,
                    d.last_seen_at,
                    d.last_call_at,
                    d.last_output_path,
                    'current',
                    ''
                FROM agent_defs d
                LEFT JOIN agent_instances i
                    ON i.agent_key = d.agent_key
                    AND i.state_path = d.state_path
                WHERE d.agent_key = ?1
                "#,
                [agent_key],
                current_role_from_row,
            )
            .optional()
            .ok()
            .flatten();
    }

    let suffix = activity_id.strip_prefix(historical_prefix)?;
    if suffix.is_empty() || suffix.contains('/') || suffix.contains('\\') {
        return None;
    }
    let records = query_historical_instances(&connection).ok()?;
    records
        .into_iter()
        .find(|record| historical_activity_suffix(record) == suffix)
}

pub fn historical_activity_suffix(record: &AgentRegistryRecord) -> String {
    let identity = record
        .instance_id
        .as_deref()
        .filter(|value| !value.is_empty())
        .unwrap_or(&record.state_path);
    short_hex(identity.as_bytes())
}

fn current_role_from_row(row: &Row<'_>) -> rusqlite::Result<AgentRegistryRecord> {
    Ok(AgentRegistryRecord {
        agent_key: row.get(0)?,
        instance_id: non_empty(row.get::<_, String>(1)?),
        public_name: row.get(2)?,
        state_path: row.get(3)?,
        backend: row.get(4)?,
        harness: row.get(5)?,
        tier: row.get(6)?,
        model: row.get(7)?,
        effort: row.get(8)?,
        session_id: row.get(9)?,
        status: row.get(10)?,
        created_at: row.get(11)?,
        updated_at: row.get(12)?,
        last_seen_at: row.get(13)?,
        last_call_at: row.get(14)?,
        last_output_path: row.get(15)?,
        cleanup_state: row.get(16)?,
        cleanup_error: row.get(17)?,
    })
}

fn historical_instance_from_row(row: &Row<'_>) -> rusqlite::Result<AgentRegistryRecord> {
    Ok(AgentRegistryRecord {
        instance_id: non_empty(row.get::<_, String>(0)?),
        agent_key: row.get(1)?,
        public_name: row.get(2)?,
        state_path: row.get(3)?,
        backend: row.get(4)?,
        harness: row.get(5)?,
        tier: row.get(6)?,
        model: row.get(7)?,
        effort: row.get(8)?,
        session_id: row.get(9)?,
        status: row.get(10)?,
        created_at: row.get(11)?,
        updated_at: row.get(12)?,
        last_seen_at: row.get(13)?,
        last_call_at: row.get(14)?,
        last_output_path: row.get(15)?,
        cleanup_state: row.get(16)?,
        cleanup_error: row.get(17)?,
    })
}

fn non_empty(value: String) -> Option<String> {
    (!value.is_empty()).then_some(value)
}

fn short_hex(bytes: &[u8]) -> String {
    let digest = Sha256::digest(bytes);
    digest[..8]
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}
