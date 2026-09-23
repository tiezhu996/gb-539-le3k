#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE_URL:-http://localhost:19539}"
python3 - "$BASE" <<'PY'
import json, sys, urllib.request, urllib.error
from datetime import datetime, timedelta, timezone
base=sys.argv[1]
def call(method, path, payload=None, token=None, expected=None, idempotency=None):
    body=None if payload is None else json.dumps(payload).encode()
    headers={'Content-Type':'application/json'}
    if token: headers['Authorization']='Bearer '+token
    if idempotency: headers['Idempotency-Key']=idempotency
    req=urllib.request.Request(base+path, data=body, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            value=json.loads(response.read()); status=response.status
    except urllib.error.HTTPError as error:
        status=error.code; value=json.loads(error.read())
    print(f'{method:6} {path:46} {status}')
    if expected and status not in expected: raise SystemExit(f'expected {expected}, got {status}: {value}')
    return value, status

def iso(minutes_ago=0):
    value=datetime.now(timezone.utc).replace(microsecond=0)-timedelta(minutes=minutes_ago)
    return value.isoformat().replace('+00:00','Z')

def pair(minutes_ago, surface, core):
    at=iso(minutes_ago)
    return [{'sample_position':'surface','measured_at':at,'moisture_pct':surface,'dry_bulb_c':55,'wet_bulb_c':42},
            {'sample_position':'center','measured_at':at,'moisture_pct':core,'dry_bulb_c':55,'wet_bulb_c':42}]

call('GET','/healthz',expected=[200])
call('GET','/api/v1/kilns',expected=[401])
login,_=call('POST','/api/v1/auth/login',{'email':'admin@kilncurve.local','password':'admin123'},expected=[200])
token=login['data']['token']
call('GET','/api/v1/auth/me',token=token,expected=[200])
kilns,_=call('GET','/api/v1/kilns',token=token,expected=[200]); kiln=kilns['data'][0]
call('GET','/api/v1/lots',token=token,expected=[200])
lot_payload={'lot_code':'SMOKE-'+kiln['id'][:6]+'-'+iso()[-9:].replace(':',''),'kiln_id':kiln['id'],'species':'白橡','thickness_mm':32,'volume_m3':5,'initial_moisture_pct':44,'target_moisture_pct':10,'quality_grade':'A'}
lot,_=call('POST','/api/v1/lots',lot_payload,token=token,expected=[201]); lot=lot['data']
call('GET','/api/v1/lots/'+lot['id'],token=token,expected=[200])
lot,_=call('POST','/api/v1/lots/'+lot['id']+'/transition',{'state':'conditioning','version':lot['version']},token=token,expected=[200]); lot=lot['data']
call('POST','/api/v1/lots/'+lot['id']+'/transition',{'state':'queued','version':lot['version']},token=token,expected=[409])
readings={'timber_lot_id':lot['id'],'readings':pair(4,31,38)}
call('POST','/api/v1/readings/import',readings,token=token,expected=[201])
call('GET','/api/v1/readings?lot_id='+lot['id'],token=token,expected=[200])
run_id=iso()
schedule,_=call('POST','/api/v1/schedules/calculate',{'timber_lot_id':lot['id']},token=token,expected=[201],idempotency='smoke-key-'+run_id); schedule=schedule['data']
same,_=call('POST','/api/v1/schedules/calculate',{'timber_lot_id':lot['id']},token=token,expected=[201],idempotency='smoke-key-'+run_id)
if same['data']['id'] != schedule['id']: raise SystemExit('idempotency failed')
call('GET','/api/v1/schedules/'+schedule['id'],token=token,expected=[200])
call('POST','/api/v1/schedules/'+schedule['id']+'/review',{'decision':'accepted','version':schedule['version']},token=token,expected=[403])
reviewer,_=call('POST','/api/v1/auth/login',{'email':'reviewer@kilncurve.local','password':'reviewer123'},expected=[200]); reviewer=reviewer['data']['token']
accepted,_=call('POST','/api/v1/schedules/'+schedule['id']+'/review',{'decision':'accepted','note':'边界核对完成','version':schedule['version']},token=reviewer,expected=[200]); accepted=accepted['data']
frozen,_=call('POST','/api/v1/schedules/'+schedule['id']+'/freeze',{'version':accepted['version']},token=reviewer,expected=[200]); frozen=frozen['data']

# Reading backfill expires the same-batch frozen plan but keeps its snapshot.
call('POST','/api/v1/readings/import',{'timber_lot_id':lot['id'],'readings':pair(2,27,31)},token=token,expected=[201])
stale,_=call('GET','/api/v1/schedules/'+schedule['id'],token=token,expected=[200]); stale=stale['data']
if not stale.get('expired_at') or stale.get('expiry_reason')!='reading_backfilled': raise SystemExit(f'frozen plan not expired: {stale}')
if not stale.get('frozen_snapshot', stale.get('frozen_at')): raise SystemExit('expired plan lost frozen snapshot')
call('POST','/api/v1/schedules/'+schedule['id']+'/review',{'decision':'reviewed','version':stale['version']},token=reviewer,expected=[409])
call('POST','/api/v1/schedules/'+schedule['id']+'/freeze',{'version':stale['version']},token=reviewer,expected=[409])

# Reaching equalizing without a current frozen plan retains the batch.
lot,_=call('POST','/api/v1/lots/'+lot['id']+'/transition',{'state':'drying','version':lot['version']},token=token,expected=[200]); lot=lot['data']
lot,_=call('POST','/api/v1/lots/'+lot['id']+'/transition',{'state':'equalizing','version':lot['version']},token=token,expected=[200]); lot=lot['data']
rejected,status=call('POST','/api/v1/lots/'+lot['id']+'/transition',{'state':'completed','version':lot['version']},token=token,expected=[409])
if rejected.get('error',{}).get('code')!='missing_plan': raise SystemExit(f'missing_plan code expected, got {rejected}')

# Recalculated plan points to the superseded plan and closes the relation.
current,_=call('POST','/api/v1/schedules/calculate',{'timber_lot_id':lot['id']},token=token,expected=[201]); current=current['data']
if current.get('supersedes_id')!=schedule['id']: raise SystemExit(f'successor not linked to old plan: {current}')
stale,_=call('GET','/api/v1/schedules/'+schedule['id'],token=token,expected=[200]); stale=stale['data']
if stale.get('superseded_by_id')!=current['id']: raise SystemExit(f'old plan not linked forward: {stale}')
current,_=call('POST','/api/v1/schedules/'+current['id']+'/review',{'decision':'accepted','note':'重算后边界复核','version':current['version']},token=reviewer,expected=[200]); current=current['data']
current,_=call('POST','/api/v1/schedules/'+current['id']+'/freeze',{'version':current['version']},token=reviewer,expected=[200]); current=current['data']
call('POST','/api/v1/schedules/'+current['id']+'/compare',{'baseline_schedule_id':schedule['id']},token=reviewer,expected=[200])
completed,_=call('POST','/api/v1/lots/'+lot['id']+'/transition',{'state':'completed','version':lot['version']},token=token,expected=[200]); completed=completed['data']
if completed['lot_state']!='completed': raise SystemExit('lot did not complete')

call('GET','/api/v1/audit',token=reviewer,expected=[403])
audit,_=call('GET','/api/v1/audit',token=token,expected=[200])
actions={event['action'] for event in audit['data']}
for required in ('expired','superseded','frozen','reviewed'):
    if required not in actions: raise SystemExit(f'audit missing action {required}: {actions}')
print('API smoke passed')
PY
