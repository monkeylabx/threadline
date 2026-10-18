use super::authority::{flag, string, Source};
use crate::local_interface::{InputError as Error, InputReceipt as Receipt, Intake, RunInput};
use serde_json::{json, Value};

// Declarative mapping of the canonical C1 status labels, not runtime policy.
const ERRORS: &[(&str, Error)] = &[
    ("UNAUTHENTICATED", Error::Unauthenticated),
    ("PERMISSION_DENIED", Error::PermissionDenied),
    ("ERROR_CODE_TENANT_MISMATCH", Error::TenantMismatch),
    ("ERROR_CODE_NOT_EXECUTION_OWNER", Error::NotExecutionOwner),
    ("ERROR_CODE_LEASE_LOST", Error::LeaseLost),
    ("ERROR_CODE_FENCING_TOKEN_STALE", Error::FencingTokenStale),
    ("ERROR_CODE_GRANT_REVOKED", Error::GrantRevoked),
    ("ERROR_CODE_GRANT_EXPIRED", Error::GrantExpired),
    ("UNAVAILABLE", Error::Unavailable),
    ("RESOURCE_EXHAUSTED", Error::ResourceExhausted),
    (
        "ERROR_CODE_INVALID_STATE_TRANSITION",
        Error::InvalidStateTransition,
    ),
    ("INVALID_ARGUMENT", Error::InvalidArgument),
    ("ERROR_CODE_PAYLOAD_TOO_LARGE", Error::PayloadTooLarge),
    (
        "ERROR_CODE_RUN_INPUT_OUTCOME_UNCERTAIN",
        Error::RunInputOutcomeUncertain,
    ),
    (
        "ERROR_CODE_IDEMPOTENCY_CONFLICT",
        Error::IdempotencyConflict,
    ),
];

pub fn fixture() -> Value {
    serde_json::from_str(include_str!(
        "../../../../../test/fixtures/proto/agentd-local/scenarios.json"
    ))
    .expect("canonical C1 scenarios")
}

pub fn base() -> Value {
    fixture()["base"].clone()
}

pub fn materialize(base: &Value, case: &Value) -> Value {
    let mut facts = base.clone();
    if let Some(changes) = case["overrides"].as_object() {
        for (section, values) in changes {
            facts[section]
                .as_object_mut()
                .expect("known fixture section")
                .extend(values.as_object().expect("fixture overrides").clone());
        }
    }
    if let Some(times) = facts["request"]["textRepeat"].as_u64() {
        let unit = facts["request"]["textUnit"].as_str().unwrap_or("x");
        let text = unit.repeat(usize::try_from(times).expect("bounded fixture count"));
        facts["request"]["text"] = json!(text);
    }
    facts
}

pub fn input(facts: &Value) -> RunInput {
    let input = &facts["request"];
    RunInput {
        run_id: string(input, "run").to_owned(),
        input_id: string(input, "inputId").to_owned(),
        text: string(input, "text").to_owned(),
    }
}

pub fn request(input_id: &str, text: &str) -> RunInput {
    RunInput {
        run_id: "run-a".to_owned(),
        input_id: input_id.to_owned(),
        text: text.to_owned(),
    }
}

fn seed_record(facts: &Value, index: usize) -> RunInput {
    let mut record = request(&format!("seed-{index}"), "seed");
    if index == 0 {
        if let Some(prior) = facts["intake"]["prior"].as_str() {
            record.input_id = string(&facts["request"], "inputId").to_owned();
            record.text = prior.to_owned();
        }
    }
    record
}

pub fn seeded(facts: &Value) -> Intake {
    let source = Source::base();
    let mut intake = Intake::new("run-a".to_owned());
    let state = &facts["intake"];
    let active = flag(state, "activeTurn");
    let count = usize::try_from(state["acceptedInputs"].as_u64().expect("count"))
        .expect("bounded fixture count")
        .max(usize::from(state["prior"].is_string()))
        .max(usize::from(active));
    for index in 0..count {
        let receipt = intake.accept(&source, &seed_record(facts, index));
        assert_eq!(receipt, Ok(Receipt::Accepted));
        if !active || index + 1 < count {
            intake.finish_turn();
        }
    }
    if flag(state, "uncertain") {
        intake.mark_uncertain();
    }
    intake
}

pub fn expected(label: &str) -> Result<Receipt, Error> {
    if label == "ACCEPTED" {
        return Ok(Receipt::Accepted);
    }
    if label == "DUPLICATE" {
        return Ok(Receipt::Duplicate);
    }
    Err(ERRORS
        .iter()
        .find(|entry| entry.0 == label)
        .expect("known C1 input status")
        .1)
}
