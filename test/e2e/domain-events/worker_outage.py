#!/usr/bin/env python3
"""Opt-in real PostgreSQL/NATS outage boundary probe; development fixtures only."""
import contextlib,json,os,pathlib,socket,subprocess,tempfile,time,urllib.request
ROOT=pathlib.Path(__file__).resolve().parents[3]
DSN=os.environ.get('THREADLINE_TEST_WORKER_DSN','')
PSQL=os.environ.get('THREADLINE_PG_BIN','')+'/psql'
NATS=os.environ.get('THREADLINE_TEST_NATS_BINARY','')
WORKER=os.environ.get('THREADLINE_TEST_WORKER_BIN','')

def sql(statement):
    try:return subprocess.check_output([PSQL,'-XAt',DSN,'-v','ON_ERROR_STOP=1','-c',statement],text=True,stderr=subprocess.PIPE,timeout=5).strip()
    except (OSError,subprocess.SubprocessError):raise RuntimeError('fixture database operation failed') from None

def port():
    with socket.socket() as listener:
        listener.bind(('127.0.0.1',0))
        return listener.getsockname()[1]

def wait(predicate,seconds):
    deadline=time.monotonic()+seconds
    while time.monotonic()<deadline:
        if predicate():return
        time.sleep(.05)
    raise AssertionError('bounded observation timed out')

def health(number,path):
    try:return urllib.request.urlopen(f'http://127.0.0.1:{number}/{path}',timeout=.5).status
    except urllib.error.HTTPError as error:return error.code
    except OSError:return 0

def connected(number):
    try:
        with socket.create_connection(('127.0.0.1',number),timeout=.1):return True
    except OSError:return False

def stop(process):
    if process.poll() is None:
        process.terminate()
        try:process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)

def start(stack,command,env,log):
    process=subprocess.Popen(command,env=env,stdout=log,stderr=log)
    stack.callback(stop,process)
    return process

def prepare(directory,stack):
    if subprocess.check_output([NATS,'--version'],text=True,timeout=5).strip()!='nats-server: v2.10.20':
        raise ValueError('pinned NATS 2.10.20 required')
    source=pathlib.Path(os.environ.get('THREADLINE_TEST_NATS_CONFIG',str(ROOT/'deploy/nats/nats-server.conf')))
    config=json.loads(source.read_text())
    broker_port,worker_port=port(),port()
    config.update(host='127.0.0.1',port=broker_port,http_port=port())
    config['jetstream']['store_dir']=directory+'/store'
    config_file=pathlib.Path(directory)/'nats.conf'
    config_file.write_text(json.dumps(config))
    log=stack.enter_context(open(directory+'/process.log','w'))
    command=[NATS,'-c',str(config_file)]
    broker=start(stack,command,None,log)
    wait(lambda:connected(broker_port),5)
    subprocess.run([os.sys.executable,str(ROOT/'deploy/nats/provision.py'),'--port',str(broker_port)],check=True,stdout=log,timeout=15)
    env={**os.environ,'THREADLINE_WORKER_POSTGRES_DSN':DSN,'THREADLINE_WORKER_NATS_URL':f'nats://127.0.0.1:{broker_port}',
         'THREADLINE_NATS_WORKER_USER':'threadline_worker_dev','THREADLINE_NATS_WORKER_PASSWORD':'threadline-worker-dev-only',
         'THREADLINE_OUTBOX_POLICY_FILE':str(ROOT/'deploy/compose/outbox-policy.json'),
         'THREADLINE_WORKER_OWNER_ID':'outage-driver','THREADLINE_WORKER_LISTEN_ADDRESS':f'127.0.0.1:{worker_port}'}
    worker=start(stack,[WORKER],env,log)
    wait(lambda:health(worker_port,'readyz')==200,10)
    return broker,worker,command,broker_port,worker_port,log

def exercise(fixture,stack):
    broker,worker,command,broker_port,worker_port,log=fixture
    stop(broker)
    prefix=f'outage-{os.getpid()}'
    seed=(ROOT/'services/worker/testdata/event.sql').read_text()
    for index in range(5):sql(seed.replace('$1',f"'{prefix}-{index}'"))
    attempts=f"SELECT count(*) FROM domain.outbox_delivery_attempts WHERE tenant_id LIKE '{prefix}-%'"
    wait(lambda:int(sql(attempts))>=3,25)
    count=int(sql(attempts))
    time.sleep(.7)
    assert int(sql(attempts))==count==3,'open circuit claimed more work'
    assert health(worker_port,'healthz')==200 and health(worker_port,'readyz')==503
    print('PASS outage: 3 real attempts, claims stop, liveness=200/readiness=503',flush=True)
    start(stack,command,None,log)
    wait(lambda:connected(broker_port),5)
    delivered=f"SELECT count(*) FROM domain.transactional_outbox WHERE tenant_id LIKE '{prefix}-%' AND delivery_state='delivered'"
    wait(lambda:int(sql(delivered))==5,50)
    assert worker.poll() is None and health(worker_port,'healthz')==200
    print('PASS recovery: all 5 rows delivered without restarting Worker',flush=True)

def main():
    if not urllib.parse.urlparse(DSN).path.startswith('/threadline_worker_process_test_'):
        raise ValueError('explicit disposable Worker database required')
    if not NATS or not WORKER or not os.environ.get('THREADLINE_PG_BIN'):
        raise ValueError('explicit test binaries required')
    version=sql('SHOW server_version')
    if version!='16.4' and not version.startswith('16.4.'):
        raise ValueError('pinned PostgreSQL 16.4 required')
    if sql("SELECT count(*) FROM domain.transactional_outbox WHERE delivery_state!='delivered'")!='0':
        raise ValueError('fixture has unrelated pending work; use a fresh database')
    with tempfile.TemporaryDirectory(prefix='threadline-outage-') as directory,contextlib.ExitStack() as stack:
        exercise(prepare(directory,stack),stack)

if __name__=='__main__':main()
