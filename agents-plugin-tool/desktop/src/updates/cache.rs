use std::{
    fs::{self, File},
    io::{self, Read, Write},
    path::PathBuf,
};

use rusqlite::{params, Connection, OptionalExtension};

use super::feed::{StagedMetadata, MAX_PAYLOAD};

const MAX_METADATA: usize = (1 << 20) + 1024;

pub struct Cache {
    pub stage: PathBuf,
    database: Connection,
}

impl Cache {
    pub fn open(directory: PathBuf) -> Result<Self, Box<dyn std::error::Error>> {
        fs::create_dir_all(&directory)?;
        let database = Connection::open(directory.join("preferences.sqlite"))?;
        database.execute_batch(
            "PRAGMA journal_mode = WAL;
             CREATE TABLE IF NOT EXISTS skipped (id INTEGER PRIMARY KEY, version TEXT NOT NULL);",
        )?;
        Ok(Self {
            stage: directory.join("staged-package"),
            database,
        })
    }

    pub fn skipped(&self) -> rusqlite::Result<Option<String>> {
        self.database
            .query_row("SELECT version FROM skipped WHERE id = 1", [], |row| {
                row.get(0)
            })
            .optional()
    }

    pub fn skip(&self, version: &str) -> rusqlite::Result<()> {
        self.database.execute(
            "INSERT INTO skipped VALUES (1, ?1)
             ON CONFLICT(id) DO UPDATE SET version = excluded.version
             WHERE version != excluded.version",
            params![version],
        )?;
        Ok(())
    }
}

pub fn load_header(path: &std::path::Path) -> io::Result<Option<(StagedMetadata, File)>> {
    let mut input = match File::open(path) {
        Ok(file) => file,
        Err(error) if error.kind() == io::ErrorKind::NotFound => return Ok(None),
        Err(error) => return Err(error),
    };
    let mut header = [0; 4];
    input.read_exact(&mut header)?;
    let size = u32::from_le_bytes(header) as usize;
    if size == 0 || size > MAX_METADATA {
        return Err(io::ErrorKind::InvalidData.into());
    }
    let mut metadata = vec![0; size];
    input.read_exact(&mut metadata)?;
    Ok(Some((serde_json::from_slice(&metadata)?, input)))
}

pub fn load_bytes(input: File, size: usize) -> io::Result<Vec<u8>> {
    if !(1..=MAX_PAYLOAD).contains(&size) {
        return Err(io::ErrorKind::InvalidData.into());
    }
    let mut bytes = Vec::new();
    input.take(size as u64 + 1).read_to_end(&mut bytes)?;
    if bytes.len() != size {
        return Err(io::ErrorKind::InvalidData.into());
    }
    Ok(bytes)
}

pub fn stage(path: &std::path::Path, metadata: &StagedMetadata, bytes: &[u8]) -> io::Result<()> {
    let encoded = serde_json::to_vec(metadata)?;
    if encoded.len() > MAX_METADATA {
        return Err(io::ErrorKind::InvalidData.into());
    }
    let partial = path.with_extension("partial");
    let result = (|| {
        let mut file = File::create(&partial)?;
        file.write_all(&(encoded.len() as u32).to_le_bytes())?;
        file.write_all(&encoded)?;
        file.write_all(bytes)?;
        file.sync_all()?;
        drop(file);
        fs::rename(&partial, path)
    })();
    if result.is_err() {
        let _ = fs::remove_file(partial);
    }
    result
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn skip_and_complete_stage_survive_reopen_and_incomplete_stage_does_not() {
        let directory = tempfile::tempdir().unwrap();
        let cache = Cache::open(directory.path().to_owned()).unwrap();
        assert_eq!(cache.skipped().unwrap(), None);
        cache.skip("1.0.1").unwrap();
        let metadata = serde_json::from_str(r#"{"action":"stable","manifest":{}}"#).unwrap();
        let path = cache.stage.clone();
        stage(&path, &metadata, b"package").unwrap();
        drop(cache);
        let reopened = Cache::open(directory.path().to_owned()).unwrap();
        assert_eq!(reopened.skipped().unwrap().as_deref(), Some("1.0.1"));
        let (_, input) = load_header(&path).unwrap().unwrap();
        assert_eq!(load_bytes(input, 7).unwrap(), b"package");
        let (_, input) = load_header(&path).unwrap().unwrap();
        assert!(load_bytes(input, 8).is_err());
        let (_, input) = load_header(&path).unwrap().unwrap();
        assert!(load_bytes(input, 6).is_err());
        fs::write(&path, u32::MAX.to_le_bytes()).unwrap();
        assert!(load_header(&path).is_err());
    }
}
