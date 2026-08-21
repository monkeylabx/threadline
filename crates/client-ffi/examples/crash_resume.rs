use std::env;
use std::fs::{self, OpenOptions};
use std::io::{self, Write};
use std::path::Path;
use std::process;

use threadline_client_ffi::{
    threadline_client_create, threadline_client_release, threadline_stream_next,
    threadline_stream_release, threadline_stream_start,
};

const SIMULATED_CRASH_EXIT: i32 = 86;

fn main() {
    if let Err(error) = run() {
        eprintln!("native bridge crash fixture failed: {error}");
        process::exit(1);
    }
}

fn run() -> Result<(), Box<dyn std::error::Error>> {
    let mut arguments = env::args().skip(1);
    let mode = arguments.next().ok_or("missing mode: write or resume")?;
    let cursor_path = arguments.next().ok_or("missing cursor path")?;
    if arguments.next().is_some() {
        return Err("unexpected extra arguments".into());
    }

    match mode.as_str() {
        "write" => write_then_terminate(Path::new(&cursor_path)),
        "resume" => resume(Path::new(&cursor_path)),
        _ => Err("mode must be write or resume".into()),
    }
}

fn write_then_terminate(cursor_path: &Path) -> Result<(), Box<dyn std::error::Error>> {
    let client = create_client()?;
    let stream = create_stream(client, 0, 3)?;
    let mut cursor = 0;
    for expected in 1..=3 {
        cursor = next(stream)?;
        if cursor != expected {
            return Err(format!("expected sequence {expected}, received {cursor}").into());
        }
    }

    let mut cursor_file = OpenOptions::new()
        .create(true)
        .truncate(true)
        .write(true)
        .open(cursor_path)?;
    writeln!(cursor_file, "{cursor}")?;
    cursor_file.sync_all()?;

    // Deliberately terminate without releasing native handles. The resume mode
    // runs in a fresh process and must continue from the durable cursor.
    process::exit(SIMULATED_CRASH_EXIT);
}

fn resume(cursor_path: &Path) -> Result<(), Box<dyn std::error::Error>> {
    let cursor: u64 = fs::read_to_string(cursor_path)?.trim().parse()?;
    let client = create_client()?;
    let stream = create_stream(client, cursor, 2)?;
    for expected in (cursor + 1)..=(cursor + 2) {
        let actual = next(stream)?;
        if actual != expected {
            return Err(format!("expected resumed sequence {expected}, received {actual}").into());
        }
    }
    let mut ignored = 0;
    let terminal = threadline_stream_next(stream, 2_000, &mut ignored);
    if terminal != 10 {
        return Err(format!("expected end-of-stream status 10, received {terminal}").into());
    }
    ensure_ok(threadline_stream_release(stream), "release stream")?;
    ensure_ok(threadline_client_release(client), "release client")?;
    println!(
        "resumed cursor {cursor} with sequences {} and {}",
        cursor + 1,
        cursor + 2
    );
    Ok(())
}

fn create_client() -> Result<u64, Box<dyn std::error::Error>> {
    let mut handle = 0;
    ensure_ok(threadline_client_create(&mut handle), "create client")?;
    if handle == 0 {
        return Err("client handle was zero".into());
    }
    Ok(handle)
}

fn create_stream(
    client: u64,
    cursor: u64,
    event_count: u32,
) -> Result<u64, Box<dyn std::error::Error>> {
    let mut handle = 0;
    ensure_ok(
        threadline_stream_start(client, cursor, event_count, 2, 0, 0, &mut handle),
        "start stream",
    )?;
    if handle == 0 {
        return Err("stream handle was zero".into());
    }
    Ok(handle)
}

fn next(stream: u64) -> Result<u64, Box<dyn std::error::Error>> {
    let mut sequence = 0;
    ensure_ok(
        threadline_stream_next(stream, 2_000, &mut sequence),
        "read stream event",
    )?;
    Ok(sequence)
}

fn ensure_ok(status: i32, operation: &str) -> io::Result<()> {
    if status == 0 {
        Ok(())
    } else {
        Err(io::Error::other(format!(
            "{operation} returned native status {status}"
        )))
    }
}
