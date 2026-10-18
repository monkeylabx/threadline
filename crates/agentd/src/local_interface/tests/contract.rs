use super::{authority::Source, fixtures};

#[test]
fn canonical_c1_input_scenarios_match_the_rust_intake() {
    let fixture = fixtures::fixture();
    let cases = fixture["cases"].as_array().expect("canonical cases");
    let mut checked = 0;
    for case in cases.iter().filter(|case| case["operation"] == "input") {
        let facts = fixtures::materialize(&fixture["base"], case);
        let mut intake = fixtures::seeded(&facts);
        let source = Source::new(facts.clone());
        let result = intake.accept(&source, &fixtures::input(&facts));
        let expected = fixtures::expected(case["expected"].as_str().expect("status"));
        assert_eq!(result, expected, "scenario {}", case["id"]);
        checked += 1;
    }
    assert!(checked >= 39, "the complete C1 input matrix must execute");
}
