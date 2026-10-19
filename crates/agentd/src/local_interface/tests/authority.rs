use super::fixtures;
use crate::local_interface::{
    AuthoritySource, GrantState, LookupError, Principal, RunAuthority, TrustedBinding,
};
use serde_json::Value;
use std::cell::Cell;

pub struct Source {
    pub facts: Value,
    pub lookups: Cell<usize>,
    pub forbid_lookup: bool,
}

impl Source {
    pub fn new(facts: Value) -> Self {
        Self {
            facts,
            lookups: Cell::new(0),
            forbid_lookup: false,
        }
    }

    pub fn base() -> Self {
        Self::new(fixtures::base())
    }
}

pub fn string<'a>(value: &'a Value, key: &str) -> &'a str {
    value[key].as_str().unwrap_or("")
}

pub fn flag(value: &Value, key: &str) -> bool {
    value[key].as_bool().unwrap_or(false)
}

fn grant(run: &Value) -> GrantState {
    if !flag(run, "grant") {
        return GrantState::Revoked;
    }
    match run["grantExpired"].as_bool() {
        Some(false) => GrantState::Current,
        Some(true) => GrantState::Expired,
        None => GrantState::Unavailable,
    }
}

impl AuthoritySource for Source {
    fn binding(&self) -> TrustedBinding<'_> {
        let principal = &self.facts["principal"];
        let host = &self.facts["binding"];
        TrustedBinding {
            principal: flag(principal, "present").then(|| Principal {
                tenant: string(principal, "tenant"),
                actor: string(principal, "actor"),
                device: string(principal, "device"),
                session: string(principal, "session"),
                window: string(principal, "window"),
                active: flag(principal, "active"),
                unexpired: flag(principal, "unexpired"),
            }),
            peer_authenticated: flag(host, "peerAuthenticated"),
            session: string(host, "session"),
            window: string(host, "window"),
        }
    }

    fn run(&self, _: &str) -> Result<RunAuthority<'_>, LookupError> {
        assert!(!self.forbid_lookup, "untrusted binding reached Run lookup");
        self.lookups.set(self.lookups.get() + 1);
        let run = &self.facts["run"];
        if run.is_null() {
            return Err(LookupError::Unknown);
        }
        if run["available"].as_bool() == Some(false) {
            return Err(LookupError::Unavailable);
        }
        Ok(RunAuthority {
            id: string(run, "id"),
            tenant: string(run, "tenant"),
            actor: string(run, "authorizedActor"),
            device: string(run, "authorizedDevice"),
            control: flag(run, "control"),
            observe: flag(run, "observe"),
            owner: flag(run, "owner"),
            lease: flag(run, "lease"),
            fencing: flag(run, "fencing"),
            grant: grant(run),
            route: flag(run, "route"),
            budget: flag(run, "budget"),
            live: flag(run, "live"),
        })
    }
}
