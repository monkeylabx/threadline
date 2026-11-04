#!/usr/bin/env python3
"""Opt-in two-Desktop Session process acceptance against the real backend."""
import contextlib,os,tempfile
from dataclasses import dataclass
import worker_outage as fixture
import real_chain
from desktop_peer import Peer,settings
from desktop_services import Backend,prepare

@dataclass
class Environment:
    directory:str
    stack:object
    broker:tuple
    backend:Backend

DEVICES=[('lin-jia','actor-lin-jia',0,'a1'),('meng-qing','actor-meng-qing',1,'b2')]

def push(sender,receiver,text,monitor):
    baseline=real_chain.subscriptions(monitor)
    receiver.write('wait')
    fixture.wait(lambda:real_chain.subscriptions(monitor)>baseline,10)
    assert sender.command('send '+text)=='QUEUED'
    assert sender.command('sync').startswith('SYNC 1 true')
    assert receiver.read()=='SYNC 1 true true'
    assert receiver.command('check '+text)=='MATCH 1'


def reopened_receiver(environment,first,second):
    directory,stack,broker=environment.directory,environment.stack,environment.broker
    ports=environment.backend.ports
    second.stop()
    assert first.command('send missed-during-disconnect')=='QUEUED'
    assert first.command('sync').startswith('SYNC 1 true')
    second=Peer(settings(directory,ports,DEVICES[1]),stack,broker[5])
    assert second.command('state')=='STATE 2 0 2'
    assert second.command('wait')=='SYNC 1 true true'
    assert second.command('state')=='STATE 3 0 3'
    assert second.command('check missed-during-disconnect')=='MATCH 1'
    assert second.command('sync')=='SYNC 0 true false'
    print('PASS Desktop reconnect: persisted cursor2 resumes seq3; one copy; separate encrypted stores',flush=True)
    return second


def exercise(directory,stack):
    broker=fixture.prepare(directory,stack)
    backend=prepare(directory,stack,broker)
    ports=backend.ports
    first=Peer(settings(directory,ports,DEVICES[0]),stack,broker[5])
    second=Peer(settings(directory,ports,DEVICES[1]),stack,broker[5])
    assert first.command('sync')=='SYNC 0 true false'
    assert second.command('sync')=='SYNC 0 true false'
    push(first,second,'desktop-a-to-b',ports[2])
    push(second,first,'desktop-b-to-a',ports[2])
    print('PASS Desktop two-way push: A-to-B and B-to-A wake through Realtime; message equality verified',flush=True)
    environment=Environment(directory,stack,broker,backend)
    second=reopened_receiver(environment,first,second)
    baseline=real_chain.subscriptions(ports[2])
    second.write('wait')
    fixture.wait(lambda:real_chain.subscriptions(ports[2])>baseline,10)
    backend.realtime.stop()
    assert second.read()=='SYNC 0 true false'
    assert second.command('state')=='STATE 3 0 3'
    assert first.command('send during-gateway-outage')=='QUEUED'
    assert first.command('sync').startswith('SYNC 1 true')
    backend.realtime.restart()
    assert second.command('wait')=='SYNC 1 true true'
    print('PASS Desktop interrupted stream: Core stays usable; cursor3 preserved; reconnect resumes seq4',flush=True)
    first=offline_restart(environment,first,second)
    push(second,first,'after-recovery',ports[2])
    assert first.command('state')==second.command('state')=='STATE 6 0 6'
    for text in ['desktop-a-to-b','desktop-b-to-a','missed-during-disconnect','during-gateway-outage','queued-while-offline','after-recovery']:
        assert first.command('check '+text)==second.command('check '+text)=='MATCH 1'
    assert fixture.sql("SELECT count(*) FROM domain.domain_events WHERE tenant_id='tenant-dev'")=='6'
    print('PASS Desktop convergence: both histories contiguous1..6; all6 messages equal; no duplicates',flush=True)


def offline_restart(environment,first,second):
    directory,stack,broker=environment.directory,environment.stack,environment.broker
    backend=environment.backend
    backend.core.stop()
    assert first.command('send queued-while-offline')=='QUEUED'
    assert first.command('sync').startswith('SYNC 0 false')
    assert first.command('state')=='STATE 4 1 4'
    first.stop()
    first=Peer(settings(directory,backend.ports,DEVICES[0]),stack,broker[5])
    assert first.command('state')=='STATE 4 1 4'
    backend.core.restart()
    assert first.command('sync').startswith('SYNC 1 true')
    assert second.command('wait')=='SYNC 1 true true'
    assert first.command('state')==second.command('state')=='STATE 5 0 5'
    print('PASS Desktop offline restart: pending persisted; recovery commits once at seq5',flush=True)
    return first


def main():
    import urllib.parse
    if not urllib.parse.urlparse(fixture.DSN).path.startswith('/threadline_worker_process_test_'):
        raise ValueError('explicit disposable database required')
    required=['THREADLINE_TEST_DESKTOP_PEER_BIN','THREADLINE_TEST_CORE_BIN','THREADLINE_TEST_REALTIME_BIN','THREADLINE_PG_BIN']
    if not fixture.NATS or not fixture.WORKER or not all(os.environ.get(name) for name in required):
        raise ValueError('explicit pinned test binaries required')
    if fixture.sql('SHOW server_version')!='16.4':raise ValueError('pinned PostgreSQL16.4 required')
    if fixture.sql("SELECT count(*) FROM domain.domain_events WHERE tenant_id='tenant-dev'")!='0':
        raise ValueError('fresh Slice A fixture required')
    with tempfile.TemporaryDirectory(prefix='threadline-desktop-') as directory,contextlib.ExitStack() as stack:
        exercise(directory,stack)

if __name__=='__main__':main()
