use super::{RpcMessage, StopReason, TurnError, TurnResult};
use serde_json::{json, Value};
use std::time::Duration;

const PROMPT_ID: i64 = 3;
const MAX_TEXT_BYTES: usize = 65_536;
pub(super) const MAX_UPDATES: usize = 64;

pub(super) fn request(
    session: &str,
    text: &str,
    deadline: Duration,
) -> Result<RpcMessage, TurnError> {
    if deadline.is_zero() {
        return Err(TurnError::InvalidDeadline);
    }
    if text.is_empty() || text.len() > MAX_TEXT_BYTES {
        return Err(TurnError::InvalidInput);
    }
    RpcMessage::new(
        json!({"jsonrpc":"2.0","id":PROMPT_ID,"method":"session/prompt",
        "params":{"sessionId":session,"prompt":[{"type":"text","text":text}]}}),
    )
    .map_err(TurnError::Transport)
}

pub(super) struct TextTurn<'a> {
    session: &'a str,
    text: String,
    updates: usize,
}

impl<'a> TextTurn<'a> {
    pub(super) fn new(session: &'a str) -> Self {
        Self {
            session,
            text: String::new(),
            updates: 0,
        }
    }

    pub(super) fn accept(&mut self, message: &Value) -> Result<Option<TurnResult>, TurnError> {
        if message.get("method").is_some() {
            let text = chunk(message, self.session)?;
            self.append(text)?;
            return Ok(None);
        }
        let stop_reason = finish(message)?;
        Ok(Some(TurnResult {
            text: std::mem::take(&mut self.text),
            stop_reason,
        }))
    }

    fn append(&mut self, text: &str) -> Result<(), TurnError> {
        if text.len() > 8192
            || self.text.len() + text.len() > MAX_TEXT_BYTES
            || self.updates >= MAX_UPDATES
        {
            return Err(TurnError::LimitExceeded);
        }
        self.updates += 1;
        self.text.push_str(text);
        Ok(())
    }
}

fn chunk<'a>(message: &'a Value, session: &str) -> Result<&'a str, TurnError> {
    if message.get("method").and_then(Value::as_str) != Some("session/update")
        || message.get("id").is_some()
    {
        return Err(TurnError::UnexpectedMessage);
    }
    let params = message
        .get("params")
        .and_then(Value::as_object)
        .ok_or(TurnError::MalformedPayload)?;
    if params.get("sessionId").and_then(Value::as_str) != Some(session) {
        return Err(TurnError::UnexpectedMessage);
    }
    let update = params
        .get("update")
        .and_then(Value::as_object)
        .ok_or(TurnError::MalformedPayload)?;
    if update.get("sessionUpdate").and_then(Value::as_str) != Some("agent_message_chunk") {
        return Err(TurnError::UnexpectedMessage);
    }
    let content = update
        .get("content")
        .and_then(Value::as_object)
        .ok_or(TurnError::MalformedPayload)?;
    if content.get("type").and_then(Value::as_str) != Some("text") {
        return Err(TurnError::UnexpectedMessage);
    }
    content
        .get("text")
        .and_then(Value::as_str)
        .ok_or(TurnError::MalformedPayload)
}

fn finish(message: &Value) -> Result<StopReason, TurnError> {
    if message.get("id") != Some(&json!(PROMPT_ID)) {
        return Err(TurnError::UnexpectedMessage);
    }
    if message.get("error").is_some() {
        return Err(TurnError::RemoteError);
    }
    let reason = message
        .get("result")
        .and_then(Value::as_object)
        .and_then(|result| result.get("stopReason"))
        .and_then(Value::as_str)
        .ok_or(TurnError::MalformedPayload)?;
    match reason {
        "end_turn" => Ok(StopReason::EndTurn),
        "max_tokens" => Ok(StopReason::MaxTokens),
        "max_turn_requests" => Ok(StopReason::MaxTurnRequests),
        "refusal" => Ok(StopReason::Refusal),
        "cancelled" => Ok(StopReason::Cancelled),
        _ => Err(TurnError::MalformedPayload),
    }
}
