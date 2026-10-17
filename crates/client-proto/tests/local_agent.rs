use prost::Message;
use threadline_client_proto::threadline::local_agent::v1::SubmitRunInputRequest;

#[test]
fn local_agent_wire_fixture_roundtrips() -> Result<(), prost::DecodeError> {
    // test/fixtures/proto/agentd-local/wire.json: SubmitRunInputRequest.
    let wire = b"\x0a\x05run-a\x12\x07input-a\x1a\x06h\xc3\xa9llo";
    let request = SubmitRunInputRequest::decode(wire.as_slice())?;
    assert_eq!(request.run_id, "run-a");
    assert_eq!(request.input_id, "input-a");
    assert_eq!(request.text, "héllo");
    assert_eq!(request.encode_to_vec(), wire);
    Ok(())
}
