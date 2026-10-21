#!/usr/bin/env python3
"""Opt-in real Core/Worker/NATS/Realtime transport acceptance; no model runtime."""
import base64,contextlib,hashlib,http.client,json,os,pathlib,struct,tempfile,threading,urllib.request
import worker_outage as fixture
ROOT=fixture.ROOT
TOKENS=['dev-session-lin-jia-laptop-0000000001','dev-session-meng-qing-laptop-000000001','dev-session-outsider-laptop-000000001']

def subscriptions(port):
    with urllib.request.urlopen(f'http://127.0.0.1:{port}/varz',timeout=1) as response:
        return json.load(response)['subscriptions']

def stream(port,token,after,results):
    connection=http.client.HTTPConnection('127.0.0.1',port,timeout=10)
    try:
        payload=json.dumps({'cursors':[{'channelId':'channel-product','lastAppliedSeq':str(after)}]}).encode()
        body=b'\0'+struct.pack('>I',len(payload))+payload
        headers={'Content-Type':'application/connect+json','Connect-Protocol-Version':'1','Authorization':'Bearer '+token}
        connection.request('POST','/threadline.sync.v1.SyncService/StreamEvents',body,headers)
        response=connection.getresponse()
        header=response.read(5)
        if len(header)!=5:raise AssertionError('no stream frame')
        size=struct.unpack('>I',header[1:])[0]
        if size>1048576:raise AssertionError('oversized stream frame')
        value=json.loads(response.read(size))
        results.append(value)
    finally:connection.close()

def send(port,number):
    body=b'synthetic-real-chain-'+str(number).encode()
    envelope={'eventId':f'chain-event-{number}','tenantId':'tenant-dev','channelId':'channel-product',
              'e2eeGroupId':'group-channel-product-dev','epoch':'1','category':1,
              'senderDeviceId':'device-lin-jia-laptop','senderActorId':'actor-lin-jia',
              'idempotencyKey':f'chain-key-{number}','cryptoProfile':'tl-plain-slice-a',
              'applicationCiphertext':base64.b64encode(body).decode(),
              'contentHash':base64.b64encode(hashlib.sha256(body).digest()).decode()}
    headers={'Content-Type':'application/json','Connect-Protocol-Version':'1','Authorization':'Bearer '+TOKENS[0]}
    request=urllib.request.Request(f'http://127.0.0.1:{port}/threadline.message.v1.MessageService/SendEvent',json.dumps({'envelope':envelope}).encode(),headers)
    with urllib.request.urlopen(request,timeout=5) as response:return json.load(response)

def prepare(directory,stack,broker):
    sessions=json.loads((ROOT/'deploy/compose/dev-sessions.json').read_text())
    sessions['sessions'].append({'token':TOKENS[2],'tenant_id':'tenant-dev','actor_type':'human',
                                'actor_id':'actor-outsider','device_id':'device-outsider','session_id':'session-outsider'})
    session_file=pathlib.Path(directory)/'sessions.json'
    session_file.write_text(json.dumps(sessions))
    fixture.sql("INSERT INTO domain.members(tenant_id,actor_type,actor_id,display_name,role,state) VALUES('tenant-dev',1,'actor-outsider','Synthetic outsider',4,2) ON CONFLICT DO NOTHING")
    core_port,realtime_port=fixture.port(),fixture.port()
    common={**os.environ,'THREADLINE_OUTBOX_POLICY_FILE':str(ROOT/'deploy/compose/outbox-policy.json')}
    settings=[(os.environ['THREADLINE_TEST_CORE_BIN'],{'THREADLINE_CORE_LISTEN_ADDRESS':f'127.0.0.1:{core_port}',
               'THREADLINE_CORE_POSTGRES_DSN':fixture.DSN,'THREADLINE_DEV_SESSIONS_ENABLED':'1','THREADLINE_DEV_SESSION_FIXTURES':str(session_file)}),
              (os.environ['THREADLINE_TEST_REALTIME_BIN'],{'THREADLINE_REALTIME_LISTEN_ADDRESS':f'127.0.0.1:{realtime_port}',
               'THREADLINE_REALTIME_CORE_URL':f'http://127.0.0.1:{core_port}','THREADLINE_REALTIME_NATS_URL':f'nats://127.0.0.1:{broker[3]}',
               'THREADLINE_NATS_REALTIME_USER':'threadline_realtime_dev','THREADLINE_NATS_REALTIME_PASSWORD':'threadline-worker-dev-only'})]
    for binary,config in settings:fixture.start(stack,[binary],{**common,**config},broker[5])
    fixture.wait(lambda:all(fixture.health(port,'healthz')==200 for port in (core_port,realtime_port)),10)
    monitor=json.loads((pathlib.Path(directory)/'nats.conf').read_text())['http_port']
    return core_port,realtime_port,monitor

def exercise(ports):
    core,realtime,monitor=ports
    baseline=subscriptions(monitor)
    results=[]
    threads=[threading.Thread(target=stream,args=(realtime,token,0,results)) for token in TOKENS[:2]]
    for thread in threads:thread.start()
    fixture.wait(lambda:subscriptions(monitor)>=baseline+2,10)
    first,duplicate=send(core,1),send(core,1)
    for thread in threads:thread.join(10)
    assert_first_delivery(results,first,duplicate)
    denied=[]
    stream(realtime,TOKENS[2],0,denied)
    assert denied[0]['error']['code']=='permission_denied'
    replay=[]
    thread=threading.Thread(target=stream,args=(realtime,TOKENS[1],1,replay))
    thread.start()
    send(core,2)
    thread.join(10)
    assert len(replay)==1 and int(replay[0]['batch']['nextSeq'])==2
    fixture.wait(lambda:fixture.sql("SELECT count(*) FROM domain.transactional_outbox WHERE tenant_id='tenant-dev' AND delivery_state='delivered'")=='2',10)
    assert fixture.sql("SELECT count(*) FROM domain.domain_events WHERE tenant_id='tenant-dev'")=='2'
    print('PASS real chain: two subscribers seq1; retry deduplicated; outsider denied; cursor1 resumes seq2; 2 delivered Events',flush=True)

def assert_first_delivery(results,first,duplicate):
    assert len(results)==2 and all(int(item['batch']['nextSeq'])==1 for item in results)
    assert duplicate['deduplicated'] and int(first['serverCommit']['channelSeq'])==1

def main():
    if not urllib.parse.urlparse(fixture.DSN).path.startswith('/threadline_worker_process_test_'):
        raise ValueError('explicit disposable database required')
    if not all((fixture.NATS,fixture.WORKER,os.environ.get('THREADLINE_PG_BIN'),os.environ.get('THREADLINE_TEST_CORE_BIN'),os.environ.get('THREADLINE_TEST_REALTIME_BIN'))):
        raise ValueError('explicit test binaries required')
    if not fixture.sql('SHOW server_version').startswith('16.4'):
        raise ValueError('pinned PostgreSQL 16.4 required')
    if fixture.sql("SELECT count(*) FROM domain.domain_events WHERE tenant_id='tenant-dev'")!='0':
        raise ValueError('fresh Slice A seeded fixture required')
    with tempfile.TemporaryDirectory(prefix='threadline-chain-') as directory,contextlib.ExitStack() as stack:
        broker=fixture.prepare(directory,stack)
        exercise(prepare(directory,stack,broker))

if __name__=='__main__':main()
