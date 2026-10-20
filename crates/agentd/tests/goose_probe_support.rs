//! Hermetic probe policy/observer checks run in normal Cargo CI, without Goose.
#[test]
fn synthetic_probe_support_rejects_unsafe_policy_and_false_cleanup_receipts() {
    let python = if cfg!(windows) { "python" } else { "python3" };
    let support = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("test_support");
    let result = std::process::Command::new(python)
        .args(["-I", "-B", "-m", "unittest", "discover", "-s"])
        .arg(support)
        .args(["-p", "test_goose_probe.py", "-v"])
        .output()
        .expect("Python 3 probe-support test process");
    assert!(
        result.status.success(),
        "probe support tests failed: {}",
        String::from_utf8_lossy(&result.stderr)
    );
}
