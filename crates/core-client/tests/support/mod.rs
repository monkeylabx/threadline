//! A loopback HTTP/1.1 responder that records the requests it receives and
//! answers from a scripted list. Hermetic; no Core binary involved.

use std::{
    io::{Read, Write},
    net::{TcpListener, TcpStream},
    sync::mpsc::{self, Receiver},
    thread,
};

#[derive(Clone, Debug)]
pub struct Recorded {
    pub path: String,
    pub headers: Vec<(String, String)>,
    pub body: Vec<u8>,
}

impl Recorded {
    pub fn header(&self, name: &str) -> Option<&str> {
        self.headers
            .iter()
            .find(|(key, _)| key.eq_ignore_ascii_case(name))
            .map(|(_, value)| value.as_str())
    }
}

#[derive(Clone, Debug)]
pub struct Scripted {
    pub status: u16,
    pub content_type: &'static str,
    pub body: Vec<u8>,
}

impl Scripted {
    pub fn proto(body: Vec<u8>) -> Self {
        Self {
            status: 200,
            content_type: "application/proto",
            body,
        }
    }

    pub fn error(status: u16, code: &str) -> Self {
        Self {
            status,
            content_type: "application/json",
            body: format!(r#"{{"code":"{code}","message":"synthetic"}}"#).into_bytes(),
        }
    }
}

pub struct FakeCore {
    pub base_url: String,
    received: Receiver<Recorded>,
}

impl FakeCore {
    /// Serves exactly `responses.len()` requests, one connection each.
    pub fn start(responses: Vec<Scripted>) -> Self {
        let listener = TcpListener::bind("127.0.0.1:0").expect("bind loopback");
        let base_url = format!("http://{}", listener.local_addr().expect("addr"));
        let (sender, received) = mpsc::channel();
        thread::spawn(move || {
            for scripted in responses {
                let (stream, _) = listener.accept().expect("accept");
                let request = serve_one(stream, &scripted);
                sender.send(request).expect("record");
            }
        });
        Self { base_url, received }
    }

    pub fn next_request(&self) -> Recorded {
        self.received.recv().expect("one recorded request")
    }
}

fn serve_one(mut stream: TcpStream, scripted: &Scripted) -> Recorded {
    let mut buffer = Vec::new();
    let mut chunk = [0u8; 4096];
    let header_end = loop {
        let read = stream.read(&mut chunk).expect("read");
        buffer.extend_from_slice(&chunk[..read]);
        if let Some(position) = find_header_end(&buffer) {
            break position;
        }
        assert!(read > 0, "connection closed before headers");
    };
    let head = String::from_utf8_lossy(&buffer[..header_end]).into_owned();
    let mut lines = head.split("\r\n");
    let path = lines
        .next()
        .and_then(|line| line.split(' ').nth(1))
        .expect("request path")
        .to_owned();
    let headers: Vec<(String, String)> = lines
        .filter_map(|line| line.split_once(": "))
        .map(|(key, value)| (key.to_owned(), value.to_owned()))
        .collect();
    let length: usize = headers
        .iter()
        .find(|(key, _)| key.eq_ignore_ascii_case("content-length"))
        .map(|(_, value)| value.parse().expect("content-length"))
        .unwrap_or(0);
    let mut body = buffer[header_end + 4..].to_vec();
    while body.len() < length {
        let read = stream.read(&mut chunk).expect("read body");
        assert!(read > 0, "connection closed before body");
        body.extend_from_slice(&chunk[..read]);
    }
    let response = format!(
        "HTTP/1.1 {} X\r\nContent-Type: {}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
        scripted.status,
        scripted.content_type,
        scripted.body.len()
    );
    stream.write_all(response.as_bytes()).expect("write head");
    stream.write_all(&scripted.body).expect("write body");
    stream.flush().expect("flush");
    Recorded {
        path,
        headers,
        body,
    }
}

fn find_header_end(buffer: &[u8]) -> Option<usize> {
    buffer.windows(4).position(|window| window == b"\r\n\r\n")
}
