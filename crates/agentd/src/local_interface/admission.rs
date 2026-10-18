use super::{binding, AuthoritySource, GrantState, InputError as Error, RunAuthority};

fn grant(state: GrantState) -> Result<(), Error> {
    match state {
        GrantState::Current => Ok(()),
        GrantState::Revoked => Err(Error::GrantRevoked),
        GrantState::Expired => Err(Error::GrantExpired),
        GrantState::Unavailable => Err(Error::Unavailable),
    }
}

fn effect(run: RunAuthority<'_>) -> Result<(), Error> {
    if !run.owner {
        return Err(Error::NotExecutionOwner);
    }
    if !run.lease {
        return Err(Error::LeaseLost);
    }
    if !run.fencing {
        return Err(Error::FencingTokenStale);
    }
    grant(run.grant)?;
    if !run.route {
        return Err(Error::Unavailable);
    }
    if !run.budget {
        return Err(Error::ResourceExhausted);
    }
    if !run.live {
        return Err(Error::InvalidStateTransition);
    }
    Ok(())
}

pub(super) fn check_input(source: &dyn AuthoritySource, run_id: &str) -> Result<(), Error> {
    effect(binding::controlled_run(source, run_id)?)
}
