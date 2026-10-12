use super::TransportError;
use serde_json::{Map, Value};
use std::fmt;

/// Opaque internal data. Debug deliberately omits the worker payload.
pub struct RpcMessage(Value);

impl RpcMessage {
    pub fn new(value: Value) -> Result<Self, TransportError> {
        let object = value.as_object().ok_or(TransportError::Malformed)?;
        if object.get("jsonrpc").and_then(Value::as_str) != Some("2.0") {
            return Err(TransportError::Malformed);
        }
        if object.contains_key("method") {
            validate_call(object)?;
        } else {
            validate_response(object)?;
        }
        Ok(Self(value))
    }

    pub fn value(&self) -> &Value {
        &self.0
    }
}

impl fmt::Debug for RpcMessage {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("RpcMessage").finish_non_exhaustive()
    }
}

fn valid_id(id: &Value) -> bool {
    id.is_string() || id.as_i64().is_some()
}

fn validate_call(object: &Map<String, Value>) -> Result<(), TransportError> {
    let invalid = !object["method"].is_string()
        || object.contains_key("result")
        || object.contains_key("error")
        || object.get("id").is_some_and(|id| !valid_id(id))
        || object
            .get("params")
            .is_some_and(|params| !params.is_object() && !params.is_array());
    if invalid {
        return Err(TransportError::Malformed);
    }
    Ok(())
}

fn validate_response(object: &Map<String, Value>) -> Result<(), TransportError> {
    let id = object.get("id").ok_or(TransportError::Malformed)?;
    let error = object.get("error");
    if !(valid_id(id) || (id.is_null() && error.is_some()))
        || object.contains_key("params")
        || object.contains_key("result") == error.is_some()
    {
        return Err(TransportError::Malformed);
    }
    if let Some(error) = error {
        if error.get("code").and_then(Value::as_i64).is_none()
            || error.get("message").and_then(Value::as_str).is_none()
        {
            return Err(TransportError::Malformed);
        }
    }
    Ok(())
}
