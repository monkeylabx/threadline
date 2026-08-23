#[test]
fn bundled_test_sqlite_meets_the_production_version_floor() {
    const MINIMUM_SQLITE_VERSION: i32 = 3_051_003;

    assert!(
        rusqlite::version_number() >= MINIMUM_SQLITE_VERSION,
        "the synthetic test backend must not be older than SQLite 3.51.3"
    );
}
