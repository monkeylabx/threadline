use super::{AuthoritySource, InputError as Error, LookupError, Principal, RunAuthority};

fn valid_identity(principal: Principal<'_>) -> bool {
    principal.active
        && principal.unexpired
        && !principal.tenant.is_empty()
        && !principal.actor.is_empty()
        && !principal.device.is_empty()
        && !principal.session.is_empty()
        && !principal.window.is_empty()
}

fn principal(source: &dyn AuthoritySource) -> Result<Principal<'_>, Error> {
    let binding = source.binding();
    let principal = binding.principal.ok_or(Error::Unauthenticated)?;
    if !binding.peer_authenticated || !valid_identity(principal) {
        return Err(Error::Unauthenticated);
    }
    if principal.session != binding.session || principal.window != binding.window {
        return Err(Error::PermissionDenied);
    }
    Ok(principal)
}

fn lookup_error(error: LookupError) -> Error {
    match error {
        LookupError::Unknown => Error::PermissionDenied,
        LookupError::Unavailable => Error::Unavailable,
    }
}

pub(super) fn controlled_run<'a>(
    source: &'a dyn AuthoritySource,
    run_id: &str,
) -> Result<RunAuthority<'a>, Error> {
    let principal = principal(source)?;
    if run_id.is_empty() {
        return Err(Error::PermissionDenied);
    }
    let run = source.run(run_id).map_err(lookup_error)?;
    if run.id != run_id {
        return Err(Error::PermissionDenied);
    }
    if principal.tenant != run.tenant {
        return Err(Error::TenantMismatch);
    }
    if principal.actor != run.actor || principal.device != run.device || !run.control {
        return Err(Error::PermissionDenied);
    }
    Ok(run)
}
