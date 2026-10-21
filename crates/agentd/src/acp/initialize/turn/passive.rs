//! Pinned passive observations only: validate, discard, never grant authority.
use super::TurnError;
use serde_json::{Map, Value};

type Object = Map<String, Value>;
const MAX_PASSIVE_FRAME_BYTES: usize = 16_384;

pub(super) fn validate(update: &Value, bytes: usize) -> Result<(), TurnError> {
    if bytes > MAX_PASSIVE_FRAME_BYTES {
        return Err(TurnError::LimitExceeded);
    }
    let valid = match update.get("sessionUpdate").and_then(Value::as_str) {
        Some("usage_update") => usage(update),
        Some("available_commands_update") => commands(update),
        Some("session_info_update") => info(update),
        _ => return Err(TurnError::UnexpectedMessage),
    };
    if !valid {
        return Err(TurnError::MalformedPayload);
    }
    Ok(())
}

fn shape(value: &Value, keys: &[&str]) -> bool {
    value.as_object().is_some_and(|object| {
        object.keys().all(|key| keys.contains(&key.as_str()))
            && object.get("_meta").is_none_or(Value::is_object)
    })
}

fn string(value: Option<&Value>, max: usize, nonempty: bool) -> bool {
    value
        .and_then(Value::as_str)
        .is_some_and(|text| text.len() <= max && (!nonempty || !text.is_empty()))
}

fn usage(update: &Value) -> bool {
    shape(update, &["sessionUpdate", "used", "size", "cost", "_meta"])
        && update.get("used").and_then(Value::as_u64).is_some()
        && update.get("size").and_then(Value::as_u64).is_some()
        && update.get("cost").is_none_or(cost)
}

fn cost(value: &Value) -> bool {
    shape(value, &["amount", "currency", "_meta"])
        && value
            .get("amount")
            .and_then(Value::as_f64)
            .is_some_and(|amount| amount.is_finite() && amount >= 0.0)
        && string(value.get("currency"), 16, true)
}

fn commands(update: &Value) -> bool {
    shape(update, &["sessionUpdate", "availableCommands", "_meta"])
        && update
            .get("availableCommands")
            .and_then(Value::as_array)
            .is_some_and(|items| items.len() <= 32 && items.iter().all(command))
}

fn command(value: &Value) -> bool {
    shape(value, &["name", "description", "input", "_meta"])
        && string(value.get("name"), 256, true)
        && string(value.get("description"), 4096, false)
        && value.get("input").is_none_or(|input| {
            shape(input, &["hint", "_meta"]) && string(input.get("hint"), 1024, false)
        })
}

fn info(update: &Value) -> bool {
    if !shape(update, &["sessionUpdate", "_meta"]) {
        return false;
    }
    let Some(meta) = update.get("_meta").and_then(Value::as_object) else {
        return false;
    };
    let Some(goose) = meta.get("goose").and_then(Value::as_object) else {
        return false;
    };
    meta.len() == 1 && goose.len() == 1 && active_run(goose)
}

fn active_run(goose: &Object) -> bool {
    match goose.get("activeRunId") {
        Some(Value::Null) => true,
        Some(Value::String(id)) => {
            !id.is_empty() && id.len() <= 256 && !id.chars().any(char::is_control)
        }
        _ => false,
    }
}
