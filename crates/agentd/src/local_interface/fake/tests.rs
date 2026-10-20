use std::future::Future;

mod cleanup_driver;
mod core;
mod flow;
mod io;
mod lifecycle;
mod restart;
mod stop_auth;
mod stop_flow;
mod stop_report;
mod view;
mod wire;

fn run(future: impl Future<Output = ()>) {
    tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .start_paused(true)
        .build()
        .expect("hermetic runtime")
        .block_on(future);
}
