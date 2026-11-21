use std::time::Duration;

use reqwest::{redirect::Policy, Client, StatusCode};
use semver::Version;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use tauri::{utils::platform::bundle_type, AppHandle};
use tauri_plugin_updater::{Update, UpdaterExt};
use url::Url;

const REPO: &str = "kang-sw/werkschaft";
const GITHUB: &str = "https://github.com";
const EVALUATION: &str = "http://127.0.0.1:43137";
const MANIFEST: &str = "werkschaft-tauri-update.json";
const MAX_JSON: usize = 1 << 20;
pub const MAX_PAYLOAD: usize = 256 << 20;
const PAGES: usize = 10;
const PAGE_SIZE: usize = 30;

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "kebab-case")]
pub enum Action {
    Stable,
    Canary,
    StableReturn,
}

impl Action {
    pub fn label(self) -> Option<&'static str> {
        match self {
            Self::Stable => None,
            Self::Canary => Some("canary"),
            Self::StableReturn => Some("stable-return"),
        }
    }

    fn matches(self, version: &Version, current: &Version) -> bool {
        match self {
            Self::Stable => version.pre.is_empty() && version > current,
            Self::Canary => canary(version) && version > current,
            Self::StableReturn => version.pre.is_empty() && canary(current),
        }
    }
}

fn version(text: &str) -> Option<Version> {
    let value = Version::parse(text).ok()?;
    (value.build.is_empty() && (value.pre.is_empty() || canary(&value))).then_some(value)
}

pub fn canary(version: &Version) -> bool {
    version
        .pre
        .as_str()
        .strip_prefix("canary.")
        .is_some_and(|n| n.parse::<u64>().is_ok() && n.bytes().all(|b| b.is_ascii_digit()))
}

#[derive(Clone, Copy)]
pub struct Platform {
    pub target: &'static str,
    asset: &'static str,
}

impl Platform {
    pub fn installed() -> Option<Self> {
        use tauri::utils::config::BundleType;
        let bundle = bundle_type()?;
        match bundle {
            BundleType::App if cfg!(all(target_os = "macos", target_arch = "aarch64")) => {
                Some(Self {
                    target: "darwin-aarch64",
                    asset: "Werkschaft-darwin-arm64.app.tar.gz",
                })
            }
            BundleType::Nsis if cfg!(all(windows, target_arch = "x86_64")) => Some(Self {
                target: "windows-x86_64-nsis",
                asset: "Werkschaft-win32-x64-setup.exe",
            }),
            BundleType::Msi if cfg!(all(windows, target_arch = "x86_64")) => Some(Self {
                target: "windows-x86_64-msi",
                asset: "Werkschaft-win32-x64.msi",
            }),
            _ => None,
        }
    }
}

fn asset_url(root: &str, tag: &str, asset: &str) -> String {
    format!("{root}/{REPO}/releases/download/{tag}/{asset}")
}

pub fn release_page(version: &str) -> String {
    format!("{GITHUB}/{REPO}/releases/tag/v{version}")
}

#[derive(Deserialize)]
struct Asset {
    name: String,
    browser_download_url: String,
}

#[derive(Deserialize)]
struct Release {
    tag_name: String,
    prerelease: bool,
    draft: bool,
    #[serde(default)]
    assets: Vec<Asset>,
}

impl Release {
    fn has(&self, name: &str) -> bool {
        let expected = asset_url(GITHUB, &self.tag_name, name);
        self.assets
            .iter()
            .any(|asset| asset.name == name && asset.browser_download_url == expected)
    }

    fn eligible(&self, platform: Platform, prerelease: bool) -> Option<Version> {
        let value = version(self.tag_name.strip_prefix('v')?)?;
        (!self.draft
            && self.prerelease == prerelease
            && canary(&value) == prerelease
            && self.has(MANIFEST)
            && self.has(platform.asset)
            && self.has(&format!("{}.sig", platform.asset)))
        .then_some(value)
    }
}

#[derive(Deserialize)]
struct Integrity {
    url: String,
    signature: String,
    sha256: String,
    size: usize,
}

impl Integrity {
    fn valid(&self, tag: &str, platform: Platform) -> bool {
        self.url == asset_url(GITHUB, tag, platform.asset)
            && (1..=MAX_PAYLOAD).contains(&self.size)
            && self.sha256.len() == 64
            && self
                .sha256
                .bytes()
                .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
            && (1..=4096).contains(&self.signature.len())
            && !self.signature.trim().is_empty()
    }
}

#[derive(Deserialize, Serialize)]
pub struct StagedMetadata {
    action: Action,
    manifest: serde_json::Value,
}

pub struct Offer {
    pub action: Action,
    pub update: Update,
    pub metadata: StagedMetadata,
    integrity: Integrity,
}

impl Offer {
    pub fn size(&self) -> usize {
        self.integrity.size
    }

    pub fn verify(&self, bytes: &[u8]) -> Result<(), &'static str> {
        if bytes.len() != self.integrity.size
            || format!("{:x}", Sha256::digest(bytes)) != self.integrity.sha256
        {
            return Err("The update file did not match its release checksum.");
        }
        self.update
            .verify(bytes)
            .map_err(|_| "The update signature could not be verified.")
    }
}

pub struct Feed {
    client: Client,
    api: &'static str,
    assets: &'static str,
    platform: Platform,
}

impl Feed {
    pub fn new(platform: Platform) -> Result<Self, reqwest::Error> {
        let mut builder = Client::builder()
            .user_agent("werkschaft-desktop")
            .connect_timeout(Duration::from_secs(8))
            .read_timeout(Duration::from_secs(15));
        let (api, assets) = if option_env!("WERKSCHAFT_EVALUATION") == Some("1") {
            builder = builder.no_proxy().redirect(Policy::none());
            (EVALUATION, EVALUATION)
        } else {
            builder = builder.https_only(true).redirect(Policy::limited(5));
            ("https://api.github.com", GITHUB)
        };
        Ok(Self {
            client: builder.build()?,
            api,
            assets,
            platform,
        })
    }

    async fn read(
        &self,
        url: &str,
        accept: &'static str,
        max: usize,
        timeout: Duration,
    ) -> Result<Option<Vec<u8>>, &'static str> {
        let response = self
            .client
            .get(url)
            .header("Accept", accept)
            .timeout(timeout)
            .send()
            .await
            .map_err(|_| "Could not reach the update feed.")?;
        if response.status() == StatusCode::NOT_FOUND {
            return Ok(None);
        }
        let mut response = response
            .error_for_status()
            .map_err(|_| "The update server refused the request.")?;
        if response
            .content_length()
            .is_some_and(|size| size > max as u64)
        {
            return Err("The update response exceeded its expected size.");
        }
        let mut bytes = Vec::new();
        while let Some(chunk) = response
            .chunk()
            .await
            .map_err(|_| "The update download was interrupted.")?
        {
            if chunk.len() > max - bytes.len() {
                return Err("The update response exceeded its expected size.");
            }
            bytes.extend_from_slice(&chunk);
        }
        Ok(Some(bytes))
    }

    async fn json<T: serde::de::DeserializeOwned>(
        &self,
        url: &str,
    ) -> Result<Option<T>, &'static str> {
        self.read(url, "application/json", MAX_JSON, Duration::from_secs(15))
            .await?
            .map(|bytes| {
                serde_json::from_slice(&bytes).map_err(|_| "The update metadata is invalid.")
            })
            .transpose()
    }

    pub async fn check<R: tauri::Runtime>(
        &self,
        app: &AppHandle<R>,
        action: Action,
    ) -> Result<(Option<Offer>, bool), &'static str> {
        let root = format!("{}/repos/{REPO}/releases", self.api);
        let current = &app.package_info().version;
        let selected = if action == Action::Canary {
            let mut selected = None;
            let mut limited = true;
            for page in 1..=PAGES {
                let url = format!("{root}?per_page={PAGE_SIZE}&page={page}");
                let releases: Vec<Release> = self
                    .json(&url)
                    .await?
                    .ok_or("The release list is missing.")?;
                selected = best_canary(&releases, self.platform);
                if selected.is_some() || releases.len() < PAGE_SIZE {
                    limited = false;
                    break;
                }
            }
            if selected.is_none() {
                return Ok((None, limited));
            }
            selected
        } else {
            let release = self.json::<Release>(&format!("{root}/latest")).await?;
            if let Some(release) = release.as_ref() {
                if release.has(MANIFEST)
                    && !release.prerelease
                    && !release.draft
                    && release.eligible(self.platform, false).is_none()
                {
                    return Err("The stable release is missing valid update assets.");
                }
            }
            release.and_then(|release| release.eligible(self.platform, false))
        };
        let Some(target) = selected.filter(|target| action.matches(target, current)) else {
            return Ok((None, false));
        };
        let url = asset_url(self.assets, &format!("v{target}"), MANIFEST);
        let manifest: serde_json::Value = self
            .json(&url)
            .await?
            .ok_or("The update manifest is missing.")?;
        if manifest["version"].as_str() != Some(&target.to_string()) {
            return Err("The release and update versions differ.");
        }
        Ok((
            self.restore(app, StagedMetadata { action, manifest })?,
            false,
        ))
    }

    pub fn restore<R: tauri::Runtime>(
        &self,
        app: &AppHandle<R>,
        metadata: StagedMetadata,
    ) -> Result<Option<Offer>, &'static str> {
        let target = metadata.manifest["version"]
            .as_str()
            .and_then(version)
            .ok_or("The update version is invalid.")?;
        if !metadata
            .action
            .matches(&target, &app.package_info().version)
        {
            return Ok(None);
        }
        let integrity = manifest_entry(&metadata.manifest, &target, self.platform)?;
        let action = metadata.action;
        let builder = app
            .updater_builder()
            .target(self.platform.target)
            .version_comparator(move |current, release| action.matches(&release.version, &current));
        // The app selects and bounds GitHub metadata itself. The plugin restores
        // the validated manifest and binds the signed payload to its version.
        let endpoint = Url::parse(&format!(
            "{GITHUB}/{REPO}/releases/latest/download/{MANIFEST}"
        ))
        .unwrap();
        let plugin_manifest = serde_json::json!({
            "version": target.to_string(),
            "platforms": { self.platform.target: { "url": integrity.url, "signature": integrity.signature } }
        });
        let update = builder
            .endpoints(vec![endpoint])
            .and_then(|builder| builder.build())
            .and_then(|updater| updater.restore_update(plugin_manifest))
            .map_err(|_| "The signed update metadata is invalid.")?
            .ok_or("The update target is not eligible.")?;
        Ok(Some(Offer {
            action,
            update,
            metadata,
            integrity,
        }))
    }

    pub async fn download(&self, offer: &Offer) -> Result<Vec<u8>, &'static str> {
        let url = asset_url(
            self.assets,
            &format!("v{}", offer.update.version),
            self.platform.asset,
        );
        self.read(
            &url,
            "application/octet-stream",
            offer.size(),
            Duration::from_secs(300),
        )
        .await?
        .ok_or("The update file is missing.")
    }
}

fn best_canary(releases: &[Release], platform: Platform) -> Option<Version> {
    releases
        .iter()
        .filter_map(|release| release.eligible(platform, true))
        .max()
}

fn manifest_entry(
    manifest: &serde_json::Value,
    target: &Version,
    platform: Platform,
) -> Result<Integrity, &'static str> {
    serde_json::from_value::<Integrity>(manifest["platforms"][platform.target].clone())
        .ok()
        .filter(|data| data.valid(&format!("v{target}"), platform))
        .ok_or("The release is missing valid update integrity data.")
}

#[cfg(test)]
mod tests {
    use super::*;
    use base64::{engine::general_purpose::STANDARD, Engine};
    use serde_json::json;
    use std::sync::{Arc, Mutex};
    use tauri::test::{mock_builder, mock_context, noop_assets};

    use crate::shell::Connection;
    use crate::updates::{cache, Controller, State};

    const MAC: Platform = Platform {
        target: "darwin-aarch64",
        asset: "Werkschaft-darwin-arm64.app.tar.gz",
    };

    // Tauri updater 2.14.1's public test vector signs the bytes "test" as 2.0.0.
    const KEY: &str = "untrusted comment: minisign public key: A49EF4746FA6E6D7\nRWTX5qZvdPSepDGWg2bz8n6ArQnBMwgQK1geVosorcbLzcunD+KJ+cYe";
    const SIGNATURE: &str = concat!(
        "untrusted comment: signature from tauri secret key\n",
        "RUTX5qZvdPSepPF2zsjvP/L/TQtBLRnWRvaq+4omK1UEugtQp/USrGiI1v762sDy3id3aor7bRC9ETyeot6lMHHrWoZ5hjUM1Qc=\n",
        "trusted comment: timestamp:1791649051\tfile:payload\tversion:2.0.0\n",
        "DDq2CLOEl5cQmC2eJRHlnZPVViOZsu1CjpPKQoei4A/XBfkDqGO0B+QMrcrHgEQ5qV3mne75tdFWhJpqUXh9AA==",
    );

    #[test]
    fn signed_staging_and_interrupted_channel_switch_use_the_same_recovery_state() {
        let mut context = mock_context(noop_assets());
        context.package_info_mut().version = version("2.0.1-canary.1").unwrap();
        context.config_mut().plugins.0.insert(
            "updater".into(),
            json!({"pubkey": STANDARD.encode(KEY), "requireSignedVersion": true}),
        );
        let app = mock_builder()
            .plugin(tauri_plugin_updater::Builder::new().build())
            .build(context)
            .unwrap();
        let controller = || Controller {
            connection: Arc::new(Connection::default()),
            app: app.handle().clone(),
            feed: Feed::new(MAC).unwrap(),
            state: Mutex::new(State::default()),
            cache: Mutex::new(None),
        };
        let directory = tempfile::tempdir().unwrap();
        let stage = directory.path().join("staged-package");
        let metadata = |version: &str, action, signature: &str| StagedMetadata {
            action,
            manifest: json!({
                "version": version,
                "platforms": { MAC.target: {
                    "url": asset_url(GITHUB, &format!("v{version}"), MAC.asset),
                    "sha256": format!("{:x}", Sha256::digest(b"test")),
                    "signature": signature,
                    "size": 4
                }}
            }),
        };
        let signature = STANDARD.encode(SIGNATURE);
        let running = controller();
        running.restore(directory.path().to_owned());
        let offer = |version, action, signature| {
            running
                .feed
                .restore(app.handle(), metadata(version, action, signature))
                .unwrap()
                .unwrap()
        };
        let corrupt = offer("2.0.0", Action::StableReturn, "invalid-signature");
        let signed = offer("2.0.0", Action::StableReturn, &signature);
        assert!(running.verify_and_stage(&corrupt, b"test").is_err());
        assert!(running.verify_and_stage(&signed, b"tes").is_err());
        assert!(running.verify_and_stage(&signed, b"tAst").is_err());
        assert!(cache::load_header(&stage).unwrap().is_none());
        running.verify_and_stage(&signed, b"test").unwrap();
        drop(running);

        let reopened = controller();
        reopened.restore(directory.path().to_owned());
        assert_eq!(reopened.state.lock().unwrap().phase()["phase"], "ready");
        let canary = reopened
            .feed
            .restore(
                app.handle(),
                metadata("2.0.2-canary.1", Action::Canary, &signature),
            )
            .unwrap()
            .unwrap();
        assert!(!reopened.accept_offer(canary).unwrap());
        assert_eq!(reopened.state.lock().unwrap().phase()["action"], "canary");
        assert!(cache::load_header(&stage).unwrap().is_none());
        drop(reopened);

        let retried = controller();
        retried.restore(directory.path().to_owned());
        assert_eq!(retried.state.lock().unwrap().phase()["phase"], "unchecked");
        let signed = retried
            .feed
            .restore(
                app.handle(),
                metadata("2.0.0", Action::StableReturn, &signature),
            )
            .unwrap()
            .unwrap();
        retried.verify_and_stage(&signed, b"test").unwrap();
        assert!(cache::load_header(&stage).unwrap().is_some());
    }

    fn release(tag: &str, prerelease: bool, complete: bool) -> Release {
        Release {
            tag_name: tag.into(),
            prerelease,
            draft: false,
            assets: if complete {
                [
                    MANIFEST.into(),
                    MAC.asset.into(),
                    format!("{}.sig", MAC.asset),
                ]
                .into_iter()
                .map(|name| Asset {
                    browser_download_url: asset_url(GITHUB, tag, &name),
                    name,
                })
                .collect()
            } else {
                vec![]
            },
        }
    }

    #[test]
    fn strict_channels_and_page_local_canary_order() {
        let mut draft = release("v1.3.0-canary.1", true, true);
        draft.draft = true;
        let page = [
            release("v1.5.0-canary.1", false, true),
            release("v1.4.0-canary.1", true, false),
            draft,
            release("v1.2.0-canary.9", true, true),
            release("v1.2.0-canary.10", true, true),
            release("v1.2.1-beta.1", true, true),
        ];
        assert_eq!(best_canary(&page, MAC), version("1.2.0-canary.10"));
        assert!(page.iter().all(|r| r.eligible(MAC, false).is_none()));
        assert!(release("v1.2.0", false, true)
            .eligible(MAC, false)
            .is_some());
        assert!(version("1.2.0-canary.01").is_none());
        assert!(version("1.2.0-canary.1+build").is_none());
    }

    #[test]
    fn only_explicit_return_allows_older_stable() {
        let stable = version("1.2.0").unwrap();
        let newer_canary = version("1.2.1-canary.1").unwrap();
        let same_canary = version("1.2.0-canary.99").unwrap();
        assert!(!Action::Stable.matches(&stable, &newer_canary));
        assert!(Action::Stable.matches(&stable, &same_canary));
        assert!(Action::StableReturn.matches(&stable, &newer_canary));
        assert!(!Action::StableReturn.matches(&newer_canary, &same_canary));
        assert!(!Action::StableReturn.matches(&stable, &stable));
        assert!(!Action::Stable.matches(&newer_canary, &stable));
        assert!(Action::Canary.matches(&newer_canary, &stable));
    }

    #[test]
    fn manifest_requires_exact_platform_url_checksum_size_and_signature() {
        let target = version("1.2.0").unwrap();
        let data = json!({"url": asset_url(GITHUB, "v1.2.0", MAC.asset),
            "sha256": "a".repeat(64), "signature": "test signature", "size": 100});
        let manifest = |entry| json!({"platforms": {MAC.target: entry}});
        assert!(manifest_entry(&manifest(data.clone()), &target, MAC).is_ok());
        for field in ["url", "sha256", "size", "signature"] {
            let mut missing = data.clone();
            missing.as_object_mut().unwrap().remove(field);
            assert!(
                manifest_entry(&manifest(missing), &target, MAC).is_err(),
                "{field}"
            );
        }
        let mut wrong = data;
        wrong["url"] = "https://example.com/app.tar.gz".into();
        assert!(manifest_entry(&manifest(wrong), &target, MAC).is_err());
    }
}
