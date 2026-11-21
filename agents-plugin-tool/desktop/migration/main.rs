#![cfg_attr(windows, windows_subsystem = "windows")]

#[cfg(windows)]
const VERSION: &str = env!("WERKSCHAFT_PAYLOAD_VERSION");
#[cfg(windows)]
const ID: &str = env!("WERKSCHAFT_PAYLOAD_ID");

fn valid_payload(version: &str, id: &str) -> bool {
    !version.is_empty()
        && !version.contains(['\r', '\n', '\0'])
        && id.len() == 64
        && id.bytes().all(|c| c.is_ascii_hexdigit())
}

fn completed(marker: Option<&str>, version: &str) -> bool {
    marker == Some(version)
}

#[cfg(windows)]
mod windows {
    use super::{completed, valid_payload, ID, VERSION};
    use std::{
        ffi::{c_void, OsStr, OsString},
        fs::{self, OpenOptions},
        io::{self, Write},
        os::windows::{
            ffi::{OsStrExt, OsStringExt},
            io::AsRawHandle,
        },
        path::{Path, PathBuf},
        process::Command,
        ptr,
    };

    type Handle = *mut c_void;
    #[repr(C)]
    struct Guid {
        data1: u32,
        data2: u16,
        data3: u16,
        data4: [u8; 8],
    }
    #[repr(C)]
    struct Overlapped {
        internal: usize,
        internal_high: usize,
        offset: u32,
        offset_high: u32,
        event: Handle,
    }
    const LOCAL_APP_DATA: Guid = Guid {
        data1: 0xf1b32785,
        data2: 0x6fba,
        data3: 0x4fcf,
        data4: [0x9d, 0x55, 0x7b, 0x8e, 0x7f, 0x15, 0x70, 0x91],
    };
    const HKCU: Handle = 0x80000001u32 as i32 as isize as Handle;
    #[link(name = "shell32")]
    extern "system" {
        fn SHGetKnownFolderPath(
            id: *const Guid,
            flags: u32,
            token: Handle,
            path: *mut *mut u16,
        ) -> i32;
    }
    #[link(name = "ole32")]
    extern "system" {
        fn CoTaskMemFree(ptr: *mut c_void);
    }
    #[link(name = "kernel32")]
    extern "system" {
        fn LockFileEx(
            file: Handle,
            flags: u32,
            reserved: u32,
            low: u32,
            high: u32,
            overlap: *mut Overlapped,
        ) -> i32;
    }
    #[link(name = "advapi32")]
    extern "system" {
        fn RegGetValueW(
            key: Handle,
            subkey: *const u16,
            value: *const u16,
            flags: u32,
            kind: *mut u32,
            data: *mut c_void,
            size: *mut u32,
        ) -> i32;
    }
    #[link(name = "user32")]
    extern "system" {
        fn MessageBoxW(window: Handle, text: *const u16, caption: *const u16, flags: u32) -> i32;
    }

    fn wide(s: &OsStr) -> Vec<u16> {
        s.encode_wide().chain(Some(0)).collect()
    }
    fn wide_str(s: &str) -> Vec<u16> {
        wide(OsStr::new(s))
    }

    fn local_app_data() -> Result<PathBuf, String> {
        let mut raw = ptr::null_mut();
        let result = unsafe { SHGetKnownFolderPath(&LOCAL_APP_DATA, 0, ptr::null_mut(), &mut raw) };
        if result < 0 || raw.is_null() {
            return Err(format!(
                "Cannot locate your local app data (0x{:08X}).",
                result as u32
            ));
        }
        let mut len = 0;
        unsafe {
            while *raw.add(len) != 0 {
                len += 1;
            }
        }
        let path = PathBuf::from(OsString::from_wide(unsafe {
            std::slice::from_raw_parts(raw, len)
        }));
        unsafe {
            CoTaskMemFree(raw.cast());
        }
        Ok(path)
    }

    fn registry(name: &str) -> Result<OsString, String> {
        let key = wide_str(r"Software\Microsoft\Windows\CurrentVersion\Uninstall\Werkschaft");
        let name = wide_str(name);
        let mut data = vec![0u16; 32768];
        let mut bytes = (data.len() * 2) as u32;
        let status = unsafe {
            RegGetValueW(
                HKCU,
                key.as_ptr(),
                name.as_ptr(),
                2,
                ptr::null_mut(),
                data.as_mut_ptr().cast(),
                &mut bytes,
            )
        };
        if status != 0 {
            return Err(format!(
                "Cannot read the Werkschaft installation registration (Windows error {status})."
            ));
        }
        if bytes < 2 || bytes % 2 != 0 {
            return Err("The Werkschaft installation registration is invalid.".into());
        }
        data.truncate(bytes as usize / 2);
        if data.pop() != Some(0) || data.contains(&0) {
            return Err("The Werkschaft installation registration is invalid.".into());
        }
        Ok(OsString::from_wide(&data))
    }

    fn installed(expected: Option<&str>) -> Result<PathBuf, String> {
        let version = registry("DisplayVersion")?;
        if expected.is_some_and(|v| version != OsStr::new(v)) {
            return Err("The installer did not register the expected Werkschaft version. Please retry or reinstall Werkschaft.".into());
        }
        if registry("MainBinaryName")? != OsStr::new("werkschaft-desktop.exe") {
            return Err(
                "The registered Werkschaft executable is unexpected. Please reinstall Werkschaft."
                    .into(),
            );
        }
        let location = registry("InstallLocation")?;
        let units: Vec<u16> = location.encode_wide().collect();
        let units = if units.first() == Some(&34) && units.last() == Some(&34) && units.len() >= 2 {
            &units[1..units.len() - 1]
        } else {
            &units[..]
        };
        let dir = PathBuf::from(OsString::from_wide(units));
        if !dir.is_absolute() {
            return Err("The registered Werkschaft installation path is invalid. Please reinstall Werkschaft.".into());
        }
        let exe = dir.join("werkschaft-desktop.exe");
        if !exe.is_file() {
            return Err("The installed Werkschaft executable is missing. Please reinstall Tauri Werkschaft or Electron Werkschaft.".into());
        }
        Ok(exe)
    }

    fn run() -> Result<(), String> {
        if !valid_payload(VERSION, ID) {
            return Err(
                "This Werkschaft launcher has invalid build metadata. Please reinstall Werkschaft."
                    .into(),
            );
        }
        let dir = local_app_data()?.join("Werkschaft").join("migration");
        fs::create_dir_all(&dir).map_err(|e| format!("Cannot prepare migration state: {e}"))?;
        let lock = OpenOptions::new()
            .read(true)
            .write(true)
            .create(true)
            .open(dir.join("lock"))
            .map_err(|e| format!("Cannot open migration lock: {e}"))?;
        let mut overlap = Overlapped {
            internal: 0,
            internal_high: 0,
            offset: 0,
            offset_high: 0,
            event: ptr::null_mut(),
        };
        if unsafe { LockFileEx(lock.as_raw_handle().cast(), 2, 0, 1, 0, &mut overlap) } == 0 {
            return Err(format!(
                "Cannot lock migration state: {}",
                io::Error::last_os_error()
            ));
        }
        let marker = dir.join(ID.to_ascii_lowercase());
        let prior = match fs::read_to_string(&marker) {
            Ok(s) => Some(s),
            Err(e) if e.kind() == io::ErrorKind::NotFound => None,
            Err(e) => return Err(format!("Cannot read migration state: {e}")),
        };
        if prior
            .as_deref()
            .is_some_and(|value| !completed(Some(value), VERSION))
        {
            return Err("The migration record is invalid. Please reinstall Tauri Werkschaft or Electron Werkschaft.".into());
        }
        let exe = if completed(prior.as_deref(), VERSION) {
            installed(None).map_err(|e| {
                format!("{e}\n\nPlease reinstall Tauri Werkschaft or Electron Werkschaft.")
            })?
        } else {
            let launcher =
                std::env::current_exe().map_err(|e| format!("Cannot locate the launcher: {e}"))?;
            let installer = launcher
                .parent()
                .unwrap_or(Path::new("."))
                .join("Werkschaft-setup.exe");
            let status = Command::new(&installer).arg("/S").status().map_err(|e| format!("Cannot start the bundled installer: {e}\n\nPlease retry or reinstall Werkschaft."))?;
            if !status.success() {
                return Err(format!("Werkschaft installation failed ({status}). Please retry or reinstall Werkschaft."));
            }
            let exe = installed(Some(VERSION))?;
            // The lock excludes every payload while this durable completion is written.
            let temporary = dir.join(format!("{}.tmp", ID.to_ascii_lowercase()));
            let mut file = OpenOptions::new()
                .write(true)
                .create(true)
                .truncate(true)
                .open(&temporary)
                .map_err(|e| format!("Cannot record migration completion: {e}"))?;
            file.write_all(VERSION.as_bytes())
                .and_then(|_| file.sync_all())
                .map_err(|e| format!("Cannot record migration completion: {e}"))?;
            drop(file);
            fs::rename(temporary, &marker)
                .map_err(|e| format!("Cannot record migration completion: {e}"))?;
            exe
        };
        Command::new(exe).spawn().map_err(|e| format!("Cannot start Werkschaft: {e}\n\nPlease reinstall Tauri Werkschaft or Electron Werkschaft."))?;
        Ok(())
    }

    pub fn main() {
        if let Err(error) = run() {
            unsafe {
                MessageBoxW(
                    ptr::null_mut(),
                    wide_str(&error).as_ptr(),
                    wide_str("Werkschaft").as_ptr(),
                    0x10,
                );
            }
            std::process::exit(1);
        }
    }
}

#[cfg(windows)]
fn main() {
    windows::main();
}
#[cfg(not(windows))]
fn main() {}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn payload_metadata_is_bounded_and_digest_is_canonical() {
        assert!(valid_payload("1.2.3", &"ab".repeat(32)));
        assert!(!valid_payload("", &"ab".repeat(32)));
        assert!(!valid_payload("1\n2", &"ab".repeat(32)));
        assert!(valid_payload("1", &"AB".repeat(32)));
        assert!(!valid_payload("1", "bad"));
    }
    #[test]
    fn only_exact_completion_skips_install() {
        assert!(!completed(None, "1.0"));
        assert!(!completed(Some("1."), "1.0"));
        assert!(!completed(Some("2.0"), "1.0"));
        assert!(completed(Some("1.0"), "1.0"));
    }
}
