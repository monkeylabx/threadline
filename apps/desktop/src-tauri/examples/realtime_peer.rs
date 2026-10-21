//! Opt-in process harness using the exact Desktop Session/store path; no GUI claim.
use std::io::{self, BufRead, Write};
use threadline_core_client::CoreClient;
use threadline_desktop_host::{config::HostConfig, dto::MessageState, session::Session};
const CHANNEL: &str = "channel-product";

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let config = HostConfig::from_env(|name| std::env::var(name).ok())
        .map_err(|_| "configuration rejected")?
        .ok_or("explicit session required")?;
    let mut session = Session::connect(config).map_err(|_| "session rejected")?;
    println!("READY");
    io::stdout().flush()?;
    for line in io::stdin().lock().lines() {
        let line = line?;
        let result = command(&mut session, &line);
        match result {
            Ok(receipt) => println!("{receipt}"),
            Err(_) => println!("ERROR"),
        }
        io::stdout().flush()?;
    }
    Ok(())
}

fn command(session: &mut Session<CoreClient>, line: &str) -> Result<String, ()> {
    let (verb, body) = line.split_once(' ').unwrap_or((line, ""));
    match verb {
        "send" => {
            session.send(CHANNEL, body).map_err(|_| ())?;
            Ok("QUEUED".into())
        }
        "sync" | "wait" => {
            let woke = if verb == "wait" {
                let (client, position) = session.realtime_request(CHANNEL).map_err(|_| ())?;
                client.wait(&position).unwrap_or(false)
            } else {
                false
            };
            let report = session.sync(CHANNEL).map_err(|_| ())?;
            Ok(format!(
                "SYNC {} {} {}",
                report.applied, report.online, woke
            ))
        }
        "state" => {
            let messages = session.messages(CHANNEL).map_err(|_| ())?;
            let committed = messages
                .iter()
                .filter(|m| m.state == MessageState::Committed)
                .count();
            let cursor = session
                .applied_position(CHANNEL)
                .map_err(|_| ())?
                .after_sequence;
            Ok(format!(
                "STATE {committed} {} {cursor}",
                messages.len() - committed
            ))
        }
        "check" => Ok(format!(
            "MATCH {}",
            session
                .messages(CHANNEL)
                .map_err(|_| ())?
                .iter()
                .filter(|m| m.text == body)
                .count()
        )),
        _ => Err(()),
    }
}
