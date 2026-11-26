"""Bounded command/receipt adapter for the real Desktop Session executable."""
import os,select,subprocess,time
import worker_outage as fixture

class Peer:
    def __init__(self,settings,stack,log):
        self.process=subprocess.Popen([os.environ['THREADLINE_TEST_DESKTOP_PEER_BIN']],env=settings,
            stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=log,bufsize=0)
        stack.callback(self.stop)
        assert self.read()=='READY'

    def write(self,command):
        self.process.stdin.write((command+'\n').encode())
        self.process.stdin.flush()

    def read(self):
        deadline=time.monotonic()+35
        body=bytearray()
        while time.monotonic()<deadline:
            if not select.select([self.process.stdout],[],[],.2)[0]:continue
            byte=os.read(self.process.stdout.fileno(),1)
            if not byte:raise AssertionError('Desktop peer exited before receipt')
            if byte==b'\n':return body.decode()
            body.extend(byte)
            if len(body)>256:raise AssertionError('oversized Desktop receipt')
        raise AssertionError('Desktop receipt deadline exceeded')

    def command(self,value):
        self.write(value)
        return self.read()

    def stop(self):
        if self.process.poll() is not None:return
        self.process.stdin.close()
        try:self.process.wait(timeout=5)
        except subprocess.TimeoutExpired:fixture.stop(self.process)


def settings(directory,ports,device):
    from real_chain import TOKENS
    name,actor,token,key=device
    return {**os.environ,'THREADLINE_DESKTOP_CORE_URL':f'http://127.0.0.1:{ports[0]}',
        'THREADLINE_DESKTOP_REALTIME_URL':f'http://127.0.0.1:{ports[1]}',
        'THREADLINE_DESKTOP_TENANT_ID':'tenant-dev','THREADLINE_DESKTOP_ACTOR_ID':actor,
        'THREADLINE_DESKTOP_DEVICE_ID':f'device-{name}-laptop','THREADLINE_DESKTOP_DEV_TOKEN':TOKENS[token],
        'THREADLINE_DESKTOP_DATA_DIR':directory+'/'+name,'THREADLINE_DESKTOP_DB_KEY':key*32}
