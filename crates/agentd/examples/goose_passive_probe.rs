//! Opt-in R2 interoperability experiment, not a production launch adapter.
use serde_json::json;
use std::{error::Error, process::Stdio, time::Duration};
use threadline_agentd::acp::{stdio, Initialization};
use tokio::{
    process::{Child, Command},
    runtime::Builder,
};

fn spawn(workspace: &str, args: &[String]) -> Result<Child, Box<dyn Error>> {
    let (executable, args) = args.split_first().ok_or("missing sandbox executable")?;
    let keys = "PATH HOME TMPDIR GOOSE_PATH_ROOT GOOSE_PROVIDER GOOSE_MODEL OPENAI_HOST \
                OPENAI_API_KEY OPENAI_BASE_PATH GOOSE_MODE GOOSE_DISABLE_KEYRING \
                NO_PROXY no_proxy TERM";
    let env = keys
        .split_ascii_whitespace()
        .filter_map(|key| std::env::var_os(key).map(|value| (key, value)));
    Ok(Command::new(executable)
        .args(args)
        .env_clear()
        .envs(env)
        .current_dir(workspace)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(std::fs::File::create(
            std::path::Path::new(&workspace).join("goose.stderr.log"),
        )?)
        .kill_on_drop(true)
        .spawn()?)
}

async fn probe() -> Result<(), Box<dyn Error>> {
    let args: Vec<_> = std::env::args().skip(1).collect();
    let (workspace, args) = args.split_first().ok_or("missing synthetic workspace")?;
    let mut child = spawn(workspace, args)?;
    let deadline = Duration::from_secs(20);
    let outcome = async {
        let (reader, writer) = stdio(
            child.stdout.take().ok_or("missing stdout")?,
            child.stdin.take().ok_or("missing stdin")?,
            deadline,
        )?;
        let mut connection = Initialization::new(reader, writer, deadline)?;
        connection.initialize().await?;
        connection.create_session(workspace, deadline).await?;
        let turn = connection.prompt("synthetic fixture", deadline).await?;
        if turn.text() != "synthetic probe complete"
            || turn.stop_reason() != threadline_agentd::acp::StopReason::EndTurn
        {
            return Err("synthetic text or stop reason mismatch".into());
        }
        Ok::<_, Box<dyn Error>>(json!({"initialized": true, "session_created": true,
            "text_matches": true, "text_bytes": turn.text().len(), "stop_reason": "end_turn"}))
    }
    .await;
    // Direct-child wait only. This does not assert descendant containment.
    child.start_kill()?;
    tokio::time::timeout(deadline, child.wait()).await??;
    println!("{}", outcome?);
    Ok(())
}

fn main() -> Result<(), Box<dyn Error>> {
    Builder::new_current_thread()
        .enable_all()
        .build()?
        .block_on(probe())
}
