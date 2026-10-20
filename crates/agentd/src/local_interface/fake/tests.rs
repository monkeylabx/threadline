use std::future::Future;

mod flow;
mod io;
mod lifecycle;
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
