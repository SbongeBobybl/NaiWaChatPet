#[cfg(not(windows))]
compile_error!("deskbox-audio-session-fixture is Windows-only");

use std::{
    env, fs,
    path::{Path, PathBuf},
    process, thread,
    time::Duration,
};

use windows::{
    Win32::{
        Foundation::{CloseHandle, HANDLE, WAIT_OBJECT_0, WAIT_TIMEOUT},
        Media::Audio::{PlaySoundW, SND_ASYNC, SND_FILENAME, SND_FLAGS, SND_LOOP, SND_NODEFAULT},
        System::Threading::{OpenProcess, PROCESS_SYNCHRONIZE, WaitForSingleObject},
    },
    core::PCWSTR,
};

const SAMPLE_RATE: u32 = 8_000;
const CHANNEL_COUNT: u16 = 1;
const BITS_PER_SAMPLE: u16 = 16;
const SILENCE_SECONDS: u32 = 1;

struct HandleGuard(HANDLE);

impl Drop for HandleGuard {
    fn drop(&mut self) {
        // SAFETY: This guard owns a handle returned by OpenProcess.
        let _ = unsafe { CloseHandle(self.0) };
    }
}

struct SoundGuard;

impl Drop for SoundGuard {
    fn drop(&mut self) {
        // SAFETY: A null sound name stops asynchronous playback started by this process.
        let _ = unsafe { PlaySoundW(PCWSTR::null(), None, SND_FLAGS(0)) };
    }
}

struct FixtureArguments {
    parent_pid: u32,
    wave_path: PathBuf,
    ready_path: PathBuf,
    stop_path: PathBuf,
}

fn main() {
    if let Err(error) = run() {
        eprintln!("deskbox-audio-session-fixture: {error}");
        process::exit(2);
    }
}

fn run() -> Result<(), String> {
    let arguments = parse_arguments()?;
    require_absolute_path(&arguments.wave_path, "--wave")?;
    require_absolute_path(&arguments.ready_path, "--ready")?;
    require_absolute_path(&arguments.stop_path, "--stop")?;
    if arguments.parent_pid == 0 || arguments.parent_pid == process::id() {
        return Err("--parent-pid must identify a different live process".to_string());
    }

    // SAFETY: Access is synchronization-only and the supplied PID is validated above.
    let parent = unsafe { OpenProcess(PROCESS_SYNCHRONIZE, false, arguments.parent_pid) }
        .map(HandleGuard)
        .map_err(|error| format!("cannot bind to parent process: {error}"))?;

    if let Some(parent_directory) = arguments.wave_path.parent() {
        fs::create_dir_all(parent_directory)
            .map_err(|error| format!("cannot create wave directory: {error}"))?;
    }
    write_silent_wave(&arguments.wave_path)?;

    let wave_wide = to_null_terminated_utf16(&arguments.wave_path)?;
    // SAFETY: The absolute, NUL-terminated filename remains alive until PlaySoundW returns.
    let started = unsafe {
        PlaySoundW(
            PCWSTR(wave_wide.as_ptr()),
            None,
            SND_FILENAME | SND_ASYNC | SND_LOOP | SND_NODEFAULT,
        )
    };
    if !started.as_bool() {
        return Err("PlaySoundW rejected the silent loop".to_string());
    }
    let _sound = SoundGuard;

    thread::sleep(Duration::from_millis(250));
    if wait_for_parent(&parent) != ParentState::Running {
        return Err("parent exited before the fixture became ready".to_string());
    }
    if let Some(parent_directory) = arguments.ready_path.parent() {
        fs::create_dir_all(parent_directory)
            .map_err(|error| format!("cannot create ready directory: {error}"))?;
    }
    fs::write(
        &arguments.ready_path,
        format!(
            "pid={}\nparent_pid={}\nwave={}\n",
            process::id(),
            arguments.parent_pid,
            arguments.wave_path.display()
        ),
    )
    .map_err(|error| format!("cannot write ready marker: {error}"))?;

    while !arguments.stop_path.exists() {
        match wait_for_parent(&parent) {
            ParentState::Running => thread::sleep(Duration::from_millis(100)),
            ParentState::Exited => break,
            ParentState::Failed => return Err("parent wait failed".to_string()),
        }
    }

    let _ = fs::remove_file(&arguments.ready_path);
    Ok(())
}

#[derive(Clone, Copy, Eq, PartialEq)]
enum ParentState {
    Running,
    Exited,
    Failed,
}

fn wait_for_parent(parent: &HandleGuard) -> ParentState {
    // SAFETY: The handle remains owned by parent for the duration of the call.
    match unsafe { WaitForSingleObject(parent.0, 0) } {
        WAIT_TIMEOUT => ParentState::Running,
        WAIT_OBJECT_0 => ParentState::Exited,
        _ => ParentState::Failed,
    }
}

fn parse_arguments() -> Result<FixtureArguments, String> {
    let arguments: Vec<String> = env::args().skip(1).collect();
    let parent_pid = required_argument(&arguments, "--parent-pid")?
        .parse::<u32>()
        .map_err(|_| "--parent-pid must be an unsigned integer".to_string())?;
    Ok(FixtureArguments {
        parent_pid,
        wave_path: PathBuf::from(required_argument(&arguments, "--wave")?),
        ready_path: PathBuf::from(required_argument(&arguments, "--ready")?),
        stop_path: PathBuf::from(required_argument(&arguments, "--stop")?),
    })
}

fn required_argument(arguments: &[String], name: &str) -> Result<String, String> {
    let Some(index) = arguments.iter().position(|value| value == name) else {
        return Err(format!("missing required argument {name}"));
    };
    let Some(value) = arguments.get(index + 1) else {
        return Err(format!("missing value for {name}"));
    };
    if value.is_empty() || value.starts_with("--") {
        return Err(format!("missing value for {name}"));
    }
    Ok(value.clone())
}

fn require_absolute_path(path: &Path, argument_name: &str) -> Result<(), String> {
    if !path.is_absolute() {
        return Err(format!("{argument_name} must be an absolute path"));
    }
    Ok(())
}

fn to_null_terminated_utf16(path: &Path) -> Result<Vec<u16>, String> {
    use std::os::windows::ffi::OsStrExt;

    let mut wide: Vec<u16> = path.as_os_str().encode_wide().collect();
    if wide.contains(&0) {
        return Err("--wave contains an embedded NUL".to_string());
    }
    wide.push(0);
    Ok(wide)
}

fn write_silent_wave(path: &Path) -> Result<(), String> {
    let bytes_per_sample = u32::from(BITS_PER_SAMPLE / 8);
    let data_length = SAMPLE_RATE * u32::from(CHANNEL_COUNT) * bytes_per_sample * SILENCE_SECONDS;
    let byte_rate = SAMPLE_RATE * u32::from(CHANNEL_COUNT) * bytes_per_sample;
    let block_align = CHANNEL_COUNT * (BITS_PER_SAMPLE / 8);
    let mut wave = Vec::with_capacity((44 + data_length) as usize);
    wave.extend_from_slice(b"RIFF");
    wave.extend_from_slice(&(36 + data_length).to_le_bytes());
    wave.extend_from_slice(b"WAVE");
    wave.extend_from_slice(b"fmt ");
    wave.extend_from_slice(&16u32.to_le_bytes());
    wave.extend_from_slice(&1u16.to_le_bytes());
    wave.extend_from_slice(&CHANNEL_COUNT.to_le_bytes());
    wave.extend_from_slice(&SAMPLE_RATE.to_le_bytes());
    wave.extend_from_slice(&byte_rate.to_le_bytes());
    wave.extend_from_slice(&block_align.to_le_bytes());
    wave.extend_from_slice(&BITS_PER_SAMPLE.to_le_bytes());
    wave.extend_from_slice(b"data");
    wave.extend_from_slice(&data_length.to_le_bytes());
    wave.resize((44 + data_length) as usize, 0);
    fs::write(path, wave).map_err(|error| format!("cannot write silent wave: {error}"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn silent_wave_has_valid_pcm_header_and_zero_payload() {
        let path = env::temp_dir().join(format!(
            "deskbox-audio-session-fixture-{}-{}.wav",
            process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system clock after epoch")
                .as_nanos()
        ));
        write_silent_wave(&path).expect("write silent wave");
        let bytes = fs::read(&path).expect("read silent wave");
        let _ = fs::remove_file(path);

        assert_eq!(&bytes[0..4], b"RIFF");
        assert_eq!(&bytes[8..12], b"WAVE");
        assert_eq!(&bytes[36..40], b"data");
        assert!(bytes[44..].iter().all(|value| *value == 0));
    }

    #[test]
    fn argument_lookup_requires_an_explicit_value() {
        let arguments = vec!["--wave".to_string(), "--ready".to_string()];
        assert!(required_argument(&arguments, "--wave").is_err());
        assert!(required_argument(&arguments, "--stop").is_err());
    }
}
