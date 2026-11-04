"""Restartable real Core/Realtime process fixtures; explicit development inputs."""
from dataclasses import dataclass,field
import os
import worker_outage as fixture

@dataclass
class Launch:
    binary:str
    port:int
    env:dict=field(repr=False)

class Service:
    def __init__(self,launch,stack,log):
        self.launch,self.stack,self.log=launch,stack,log
        self.child=None
        self.restart()

    def stop(self):
        fixture.stop(self.child)

    def restart(self):
        self.child=fixture.start(self.stack,[self.launch.binary],self.launch.env,self.log)
        fixture.wait(lambda:fixture.health(self.launch.port,'healthz')==200,10)

@dataclass
class Backend:
    core:Service
    realtime:Service
    monitor:int

    @property
    def ports(self):return self.core.launch.port,self.realtime.launch.port,self.monitor


def prepare(directory,stack,broker):
    import json,pathlib
    core_port,realtime_port=fixture.port(),fixture.port()
    common={**os.environ,'THREADLINE_OUTBOX_POLICY_FILE':str(fixture.ROOT/'deploy/compose/outbox-policy.json')}
    core=Service(Launch(os.environ['THREADLINE_TEST_CORE_BIN'],core_port,{**common,
        'THREADLINE_CORE_LISTEN_ADDRESS':f'127.0.0.1:{core_port}',
        'THREADLINE_CORE_POSTGRES_DSN':fixture.DSN,'THREADLINE_DEV_SESSIONS_ENABLED':'1',
        'THREADLINE_DEV_SESSION_FIXTURES':str(fixture.ROOT/'deploy/compose/dev-sessions.json')}),stack,broker[5])
    realtime=Service(Launch(os.environ['THREADLINE_TEST_REALTIME_BIN'],realtime_port,{**common,
        'THREADLINE_REALTIME_LISTEN_ADDRESS':f'127.0.0.1:{realtime_port}',
        'THREADLINE_REALTIME_CORE_URL':f'http://127.0.0.1:{core_port}',
        'THREADLINE_REALTIME_NATS_URL':f'nats://127.0.0.1:{broker[3]}',
        'THREADLINE_NATS_REALTIME_USER':'threadline_realtime_dev',
        'THREADLINE_NATS_REALTIME_PASSWORD':'threadline-worker-dev-only'}),stack,broker[5])
    monitor=json.loads((pathlib.Path(directory)/'nats.conf').read_text())['http_port']
    return Backend(core,realtime,monitor)
