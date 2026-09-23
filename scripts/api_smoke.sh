#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE_URL:-http://localhost:19539}"
python3 - "$BASE" <<'PY'
import json, sys, urllib.request, urllib.error
from datetime import datetime, timedelta, timezone
base=sys.argv[1]
def call(method, path, payload=None, token=None, expected=None):
    body=None if payload is None else json.dumps(payload).encode()
    headers={'Content-Type':'application/json'}
    if token: headers['Authorization']='Bearer '+token
    req=urllib.request.Request(base+path, data=body, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            value=json.loads(response.read()); status=response.status
    except urllib.error.HTTPError as error:
        status=error.code; value=json.loads(error.read())
    print(f'{method:6} {path:42} {status}')
    if expected and status not in expected: raise SystemExit(f'expected {expected}, got {status}: {value}')
    return value, status

call('GET','/healthz',expected=[200])
call('GET','/api/v1/kilns',expected=[401])
login,_=call('POST','/api/v1/auth/login',{'email':'admin@kilncurve.local','password':'admin123'},expected=[200])
token=login['data']['token']
call('GET','/api/v1/auth/me',token=token,expected=[200])
kilns,_=call('GET','/api/v1/kilns',token=token,expected=[200]); kiln=kilns['data'][0]
call('GET','/api/v1/lots',token=token,expected=[200])
lot_payload={'lot_code':'SMOKE-'+kiln['id'][:6],'kiln_id':kiln['id'],'species':'白橡','thickness_mm':32,'volume_m3':5,'initial_moisture_pct':44,'target_moisture_pct':10,'quality_grade':'A'}
lot,_=call('POST','/api/v1/lots',lot_payload,token=token,expected=[201]); lot=lot['data']
call('GET','/api/v1/lots/'+lot['id'],token=token,expected=[200])
lot,_=call('POST','/api/v1/lots/'+lot['id']+'/transition',{'state':'conditioning','version':lot['version']},token=token,expected=[200]); lot=lot['data']
call('POST','/api/v1/lots/'+lot['id']+'/transition',{'state':'queued','version':lot['version']},token=token,expected=[409])
measured_at=datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace('+00:00','Z')
readings={'timber_lot_id':lot['id'],'readings':[{'sample_position':'surface','measured_at':measured_at,'moisture_pct':31,'dry_bulb_c':55,'wet_bulb_c':42},{'sample_position':'center','measured_at':measured_at,'moisture_pct':38,'dry_bulb_c':55,'wet_bulb_c':42}]}
call('POST','/api/v1/readings/import',readings,token=token,expected=[201])
call('GET','/api/v1/readings?lot_id='+lot['id'],token=token,expected=[200])
schedule,_=call('POST','/api/v1/schedules/calculate',{'timber_lot_id':lot['id']},token=token,expected=[201]); schedule=schedule['data']
same,_=call('POST','/api/v1/schedules/calculate',{'timber_lot_id':lot['id']},token=token,expected=[201])
if same['data']['id'] != schedule['id']: raise SystemExit('idempotency failed')
call('GET','/api/v1/schedules/'+schedule['id'],token=token,expected=[200])
call('POST','/api/v1/schedules/'+schedule['id']+'/review',{'decision':'accepted','version':schedule['version']},token=token,expected=[403])
reviewer,_=call('POST','/api/v1/auth/login',{'email':'reviewer@kilncurve.local','password':'reviewer123'},expected=[200]); reviewer=reviewer['data']['token']
accepted,_=call('POST','/api/v1/schedules/'+schedule['id']+'/review',{'decision':'accepted','note':'边界核对完成','version':schedule['version']},token=reviewer,expected=[200]); accepted=accepted['data']
call('POST','/api/v1/schedules/'+schedule['id']+'/freeze',{'version':accepted['version']},token=reviewer,expected=[200])
later_at=(datetime.now(timezone.utc).replace(microsecond=0)-timedelta(minutes=2)).isoformat().replace('+00:00','Z')
later_readings={'timber_lot_id':lot['id'],'readings':[{'sample_position':'surface','measured_at':later_at,'moisture_pct':27,'dry_bulb_c':53,'wet_bulb_c':42},{'sample_position':'core','measured_at':later_at,'moisture_pct':31,'dry_bulb_c':53,'wet_bulb_c':42}]}
call('POST','/api/v1/readings/import',later_readings,token=token,expected=[201])
current,_=call('POST','/api/v1/schedules/calculate',{'timber_lot_id':lot['id']},token=token,expected=[201]); current=current['data']
call('POST','/api/v1/schedules/'+current['id']+'/compare',{'baseline_schedule_id':schedule['id']},token=reviewer,expected=[200])
call('GET','/api/v1/audit',token=reviewer,expected=[403])
call('GET','/api/v1/audit',token=token,expected=[200])
print('API smoke passed')
PY
