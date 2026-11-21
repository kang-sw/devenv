use serde_json::Value;
use sha2::{Digest, Sha256};
use std::collections::BTreeSet;
use std::fs;
use std::path::{Path, PathBuf};

const INVALID: &str = "Invalid daemon package";
const STAGE: &str = "Cannot stage daemon package";
const MAX_JSON: u64 = 4 * 1024 * 1024;
const PREFIX: &str = "${CLAUDE_PLUGIN_ROOT}/";

#[derive(Debug, Clone)]
pub struct Candidate {
    pub root: PathBuf,
    pub binary: PathBuf,
    pub version: [u64; 3],
    pub config_manifests: Vec<PathBuf>,
}

fn safe_relative(value: &str) -> bool {
    !value.is_empty()
        && !value.contains(['\\', ':'])
        && value
            .split('/')
            .all(|s| !s.is_empty() && s != "." && s != "..")
}

fn plain_metadata(path: &Path) -> Result<fs::Metadata, &'static str> {
    let meta = fs::symlink_metadata(path).map_err(|_| INVALID)?;
    #[cfg(windows)]
    {
        use std::os::windows::fs::MetadataExt;
        if meta.file_attributes() & 0x400 != 0 {
            return Err(INVALID);
        }
    }
    if meta.file_type().is_symlink() {
        return Err(INVALID);
    }
    Ok(meta)
}

fn files(root: &Path) -> Result<BTreeSet<PathBuf>, &'static str> {
    fn walk(
        root: &Path,
        relative: &Path,
        result: &mut BTreeSet<PathBuf>,
    ) -> Result<(), &'static str> {
        let dir = root.join(relative);
        if !plain_metadata(&dir)?.is_dir() {
            return Err(INVALID);
        }
        for entry in fs::read_dir(dir).map_err(|_| INVALID)? {
            let entry = entry.map_err(|_| INVALID)?;
            let name = entry.file_name();
            let name = name.to_str().ok_or(INVALID)?;
            if !safe_relative(name) || name.contains('/') {
                return Err(INVALID);
            }
            let rel = relative.join(name);
            let meta = plain_metadata(&root.join(&rel))?;
            if meta.is_dir() {
                walk(root, &rel, result)?;
            } else if meta.is_file() {
                result.insert(rel);
            } else {
                return Err(INVALID);
            }
        }
        Ok(())
    }
    let mut result = BTreeSet::new();
    walk(root, Path::new(""), &mut result)?;
    Ok(result)
}

fn json(path: &Path) -> Result<Value, &'static str> {
    let meta = plain_metadata(path)?;
    if !meta.is_file() || meta.len() > MAX_JSON {
        return Err(INVALID);
    }
    serde_json::from_slice(&fs::read(path).map_err(|_| INVALID)?).map_err(|_| INVALID)
}

fn hash(bytes: &[u8]) -> String {
    format!("{:x}", Sha256::digest(bytes))
}
fn valid_hash(s: &str) -> bool {
    s.len() == 64
        && s.bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}
fn file_bytes(root: &Path, relative: &str) -> Result<Vec<u8>, &'static str> {
    if !safe_relative(relative) {
        return Err(INVALID);
    }
    let path = root.join(relative);
    if !plain_metadata(&path)?.is_file() {
        return Err(INVALID);
    }
    fs::read(path).map_err(|_| INVALID)
}
fn reference(root: &Path, value: &str) -> Result<PathBuf, &'static str> {
    let rel = value
        .strip_prefix(PREFIX)
        .or_else(|| value.strip_prefix("./"))
        .ok_or(INVALID)?;
    let bytes = file_bytes(root, rel)?;
    if rel.ends_with(".json") {
        serde_json::from_slice::<Value>(&bytes).map_err(|_| INVALID)?;
    }
    Ok(root.join(rel))
}
fn tree(root: &Path, name: &str, all: &BTreeSet<PathBuf>) -> Result<(), &'static str> {
    let dir = root.join(name);
    let manifest = json(&dir.join("manifest.json"))?;
    if manifest["schema_version"].as_u64() != Some(1) {
        return Err(INVALID);
    }
    let entries = manifest["files"]
        .as_object()
        .filter(|m| !m.is_empty())
        .ok_or(INVALID)?;
    let mut listed = BTreeSet::new();
    let mut content = false;
    for (rel, expected) in entries {
        let expected = expected.as_str().filter(|s| valid_hash(s)).ok_or(INVALID)?;
        let bytes = file_bytes(&dir, rel)?;
        let text = String::from_utf8_lossy(&bytes).replace("\r\n", "\n");
        if hash(text.as_bytes()) != expected {
            return Err(INVALID);
        }
        content |= if name == "skills" {
            rel.ends_with("/SKILL.md")
        } else {
            rel.ends_with(".md")
        };
        listed.insert(PathBuf::from(name).join(rel));
    }
    if !content {
        return Err(INVALID);
    }
    let manifest_path = PathBuf::from(name).join("manifest.json");
    for rel in all.iter().filter(|p| p.starts_with(name)) {
        if rel != &manifest_path && !listed.contains(rel) {
            return Err(INVALID);
        }
    }
    Ok(())
}

fn validate(root: &Path, development: bool) -> Result<Candidate, &'static str> {
    if !root.is_absolute() {
        return Err(INVALID);
    }
    let all = files(root)?;
    let plugin = json(&root.join(".claude-plugin/plugin.json"))?;
    let name = plugin["name"]
        .as_str()
        .filter(|s| *s == "ws" || *s == "wsflow")
        .ok_or(INVALID)?;
    let agents = plugin["agents"]
        .as_array()
        .filter(|a| !a.is_empty())
        .ok_or(INVALID)?;
    let server = &plugin["mcpServers"][name];
    let args = server["args"].as_array().ok_or(INVALID)?;
    let manifests: Vec<&str> = server["env"]["WS_MCP_CONFIG_MANIFESTS"]
        .as_str()
        .ok_or(INVALID)?
        .split(';')
        .collect();
    if !args
        .iter()
        .any(|a| a.as_str() == Some(&format!("{PREFIX}bin/ws-mcp-launcher.py")))
        || !manifests.contains(&format!("{PREFIX}config-manifest.json").as_str())
        || !manifests.contains(&format!("{PREFIX}dashboard-config-manifest.json").as_str())
    {
        return Err(INVALID);
    }
    for agent in agents {
        reference(root, agent.as_str().ok_or(INVALID)?)?;
    }
    for arg in args {
        if let Some(s) = arg.as_str() {
            if s.starts_with(PREFIX) {
                reference(root, s)?;
            }
        }
    }
    let config_manifests = manifests
        .into_iter()
        .map(|s| reference(root, s))
        .collect::<Result<Vec<_>, _>>()?;
    let contract = json(&root.join("runtime.json"))?;
    if contract["plugin"].as_str() != Some(name) {
        return Err(INVALID);
    }
    let binary = root.join(if cfg!(windows) {
        "ws-mcp.exe"
    } else {
        "ws-mcp"
    });
    let metadata = json(&root.join(if cfg!(windows) {
        "ws-mcp.exe.metadata.json"
    } else {
        "ws-mcp.metadata.json"
    }))?;
    let version = metadata["version"]
        .as_str()
        .and_then(crate::daemon::version_core)
        .ok_or(INVALID)?;
    let expected = contract["plugin_version"]
        .as_str()
        .and_then(crate::daemon::version_core)
        .ok_or(INVALID)?;
    if metadata["schema_version"].as_u64() != Some(1)
        || version[..2] != expected[..2]
        || (!development && version != expected)
    {
        return Err(INVALID);
    }
    let digest = metadata["binary_sha256"]
        .as_str()
        .filter(|s| valid_hash(s))
        .ok_or(INVALID)?;
    if !plain_metadata(&binary)?.is_file()
        || hash(&fs::read(&binary).map_err(|_| INVALID)?) != digest
    {
        return Err(INVALID);
    }
    if let Some(protocol) = metadata.get("mcp_protocol") {
        if protocol.as_str() != Some(contract["mcp_protocol"].as_str().unwrap_or("")) {
            return Err(INVALID);
        }
    }
    let exact = contract["runtime_capabilities"]["match"]
        .as_str()
        .unwrap_or("")
        .eq_ignore_ascii_case("exact");
    for field in ["tools", "commands"] {
        if let Some(actual) = metadata.get(field).filter(|v| !v.is_null()) {
            let actual = actual
                .as_array()
                .ok_or(INVALID)?
                .iter()
                .map(|v| v.as_str().ok_or(INVALID))
                .collect::<Result<BTreeSet<_>, _>>()?;
            let required: BTreeSet<&str> = contract[field]
                .as_object()
                .map(|o| o.keys().map(String::as_str).collect())
                .unwrap_or_default();
            if !required.is_subset(&actual) || (exact && actual != required) {
                return Err(INVALID);
            }
        }
    }
    tree(root, "rsrc", &all)?;
    tree(root, "skills", &all)?;
    Ok(Candidate {
        root: root.to_path_buf(),
        binary,
        version,
        config_manifests,
    })
}

impl Candidate {
    pub fn select(bundled: &Path, development: Option<&Path>) -> Result<Self, &'static str> {
        validate(development.unwrap_or(bundled), development.is_some())
    }

    pub fn stage(&self, user_data: &Path, app_resource_root: &Path) -> Result<Self, &'static str> {
        if !user_data.is_absolute() || !app_resource_root.is_absolute() {
            return Err(STAGE);
        }
        let result = || -> Result<Self, &'static str> {
            let selected = validate(&self.root, true)?;
            if selected.version != self.version
                || selected.binary != self.binary
                || selected.config_manifests != self.config_manifests
            {
                return Err(INVALID);
            }
            let container = user_data.join("daemon-stages");
            fs::create_dir_all(&container).map_err(|_| STAGE)?;
            let actual = fs::canonicalize(&container).map_err(|_| STAGE)?;
            for origin in [&self.root, app_resource_root] {
                let origin = fs::canonicalize(origin).map_err(|_| STAGE)?;
                if actual.starts_with(origin) {
                    return Err(STAGE);
                }
            }
            let partial = tempfile::Builder::new()
                .prefix("daemon-")
                .tempdir_in(&actual)
                .map_err(|_| STAGE)?;
            let before = files(&self.root)?;
            for rel in &before {
                let source = self.root.join(rel);
                let target = partial.path().join(rel);
                fs::create_dir_all(target.parent().ok_or(STAGE)?).map_err(|_| STAGE)?;
                fs::copy(&source, &target).map_err(|_| STAGE)?;
                fs::set_permissions(&target, plain_metadata(&source)?.permissions())
                    .map_err(|_| STAGE)?;
            }
            if files(&self.root)? != before || files(partial.path())? != before {
                return Err(STAGE);
            }
            for rel in &before {
                if fs::read(self.root.join(rel)).map_err(|_| STAGE)?
                    != fs::read(partial.path().join(rel)).map_err(|_| STAGE)?
                {
                    return Err(STAGE);
                }
            }
            let staged = validate(partial.path(), true)?;
            if staged.version != self.version {
                return Err(STAGE);
            }
            let root = partial.keep();
            Ok(Candidate {
                binary: root.join(
                    staged
                        .binary
                        .strip_prefix(&staged.root)
                        .map_err(|_| STAGE)?,
                ),
                config_manifests: staged
                    .config_manifests
                    .iter()
                    .map(|p| {
                        p.strip_prefix(&staged.root)
                            .map(|p| root.join(p))
                            .map_err(|_| STAGE)
                    })
                    .collect::<Result<_, _>>()?,
                root,
                version: staged.version,
            })
        };
        result().map_err(|_| STAGE)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    use std::io::Write;

    fn write(root: &Path, path: &str, bytes: &[u8]) {
        let path = root.join(path);
        fs::create_dir_all(path.parent().unwrap()).unwrap();
        fs::write(path, bytes).unwrap();
    }
    fn save(root: &Path, path: &str, value: Value) {
        write(root, path, value.to_string().as_bytes());
    }
    fn fixture(root: &Path, version: &str) {
        let bin = if cfg!(windows) {
            "ws-mcp.exe"
        } else {
            "ws-mcp"
        };
        write(root, bin, b"binary fixture");
        save(
            root,
            &format!("{bin}.metadata.json"),
            json!({"schema_version":1,"version":version,"binary_sha256":hash(b"binary fixture"),"mcp_protocol":"p","tools":["a"],"commands":["b"]}),
        );
        save(
            root,
            "runtime.json",
            json!({"plugin":"ws","plugin_version":"1.2.3","mcp_protocol":"p","tools":{"a":"*"},"commands":{"b":"*"}}),
        );
        save(
            root,
            ".claude-plugin/plugin.json",
            json!({"name":"ws","agents":["./agent.md"],"mcpServers":{"ws":{"args":[format!("{PREFIX}bin/ws-mcp-launcher.py")],"env":{"WS_MCP_CONFIG_MANIFESTS":format!("{PREFIX}config-manifest.json;{PREFIX}dashboard-config-manifest.json;{PREFIX}extra.json")}}}}),
        );
        write(root, "agent.md", b"agent");
        write(root, "bin/ws-mcp-launcher.py", b"launcher");
        for name in [
            "config-manifest.json",
            "dashboard-config-manifest.json",
            "extra.json",
        ] {
            save(root, name, json!({}));
        }
        for (tree, file) in [("rsrc", "x/a.md"), ("skills", "x/SKILL.md")] {
            write(root, &format!("{tree}/{file}"), b"hello\r\n");
            save(
                root,
                &format!("{tree}/manifest.json"),
                json!({"schema_version":1,"files":{file:hash(b"hello\n")}}),
            );
        }
    }
    #[test]
    fn selection_and_retained_stages() {
        let temp = tempfile::tempdir().unwrap();
        let bundled = temp.path().join("app/daemon");
        let dev = temp.path().join("dev");
        fixture(&bundled, "1.2.3-dev");
        fixture(&dev, "1.2.9-dev");
        let b = Candidate::select(&bundled, None).unwrap();
        assert_eq!(b.version, [1, 2, 3]);
        assert_eq!(b.config_manifests.len(), 3);
        assert!(Candidate::select(&dev, None).is_err());
        assert_eq!(
            Candidate::select(&bundled, Some(&dev)).unwrap().version,
            [1, 2, 9]
        );
        assert!(Candidate::select(&bundled, Some(&temp.path().join("missing"))).is_err());
        let user = temp.path().join("user");
        let a = b.stage(&user, &temp.path().join("app")).unwrap();
        let c = b.stage(&user, &temp.path().join("app")).unwrap();
        assert_ne!(a.root, c.root);
        assert!(b
            .stage(&temp.path().join("app"), &temp.path().join("app"))
            .is_err());
        fs::remove_dir_all(temp.path().join("app")).unwrap();
        assert_eq!(fs::read(&a.binary).unwrap(), b"binary fixture");
        assert!(a
            .config_manifests
            .iter()
            .all(|p| p.starts_with(&a.root) && p.is_file()));
        #[cfg(unix)]
        {
            use std::os::unix::fs::MetadataExt;
            assert_ne!(
                fs::metadata(&a.binary).unwrap().ino(),
                fs::metadata(&c.binary).unwrap().ino()
            );
        }
    }
    #[test]
    fn rejects_tampering_inventory_and_unsafe_references() {
        let temp = tempfile::tempdir().unwrap();
        let root = temp.path().join("daemon");
        fixture(&root, "1.2.3");
        let meta = if cfg!(windows) {
            "ws-mcp.exe.metadata.json"
        } else {
            "ws-mcp.metadata.json"
        };
        let original = fs::read(root.join(meta)).unwrap();
        let mut m: Value = serde_json::from_slice(&original).unwrap();
        m["tools"] = json!([]);
        save(&root, meta, m.clone());
        assert!(Candidate::select(&root, None).is_err());
        m["tools"] = json!(["a", "extra"]);
        save(&root, meta, m);
        let mut contract = json(&root.join("runtime.json")).unwrap();
        contract["runtime_capabilities"] = json!({"match":"exact"});
        save(&root, "runtime.json", contract);
        assert!(Candidate::select(&root, None).is_err());
        fs::write(root.join(meta), original).unwrap();
        write(&root, "rsrc/unlisted.md", b"extra");
        assert!(Candidate::select(&root, None).is_err());
        fs::remove_file(root.join("rsrc/unlisted.md")).unwrap();
        let mut plugin = json(&root.join(".claude-plugin/plugin.json")).unwrap();
        plugin["agents"] = json!(["./../outside"]);
        save(&root, ".claude-plugin/plugin.json", plugin);
        assert!(Candidate::select(&root, None).is_err());
    }
    #[test]
    fn staging_revalidates_selected_bytes() {
        let temp = tempfile::tempdir().unwrap();
        let root = temp.path().join("app/daemon");
        fixture(&root, "1.2.3");
        let candidate = Candidate::select(&root, None).unwrap();
        fs::OpenOptions::new()
            .append(true)
            .open(&candidate.binary)
            .unwrap()
            .write_all(b"tamper")
            .unwrap();
        assert!(candidate
            .stage(&temp.path().join("user"), &temp.path().join("app"))
            .is_err());
    }
    #[cfg(unix)]
    #[test]
    fn rejects_links_even_outside_manifests() {
        let temp = tempfile::tempdir().unwrap();
        let root = temp.path().join("daemon");
        fixture(&root, "1.2.3");
        std::os::unix::fs::symlink(root.join("agent.md"), root.join("extra-link")).unwrap();
        assert!(Candidate::select(&root, None).is_err());
    }
}
