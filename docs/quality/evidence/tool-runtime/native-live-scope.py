"""Actual native CLI/control-scope checks with a local, model-free HTTP fixture.

Never writes credentials, request headers, environment values or full model bodies.
"""
from pathlib import Path
import datetime,hashlib,json,os,re,shutil,subprocess,threading,uuid
from http.server import ThreadingHTTPServer,BaseHTTPRequestHandler
from urllib.parse import urlparse,urlencode
from urllib.request import Request,build_opener,HTTPRedirectHandler

ROOT=Path.cwd().resolve()
OUT=ROOT/'.cache/e2e/tool-stage'
PRIVATE=OUT/'context.local.json'
context=json.loads(PRIVATE.read_text(encoding='utf-8-sig'))
base=context['base'].rstrip('/')
project=context['project_id']
agent=context['agent_token']
runner=context['runner_token']
parsed=urlparse(base)
assert parsed.scheme=='http' and parsed.hostname in ('127.0.0.1','localhost','::1') and parsed.port==4322 and not parsed.username and not parsed.password,'unexpected_control_destination'
assert isinstance(project,str) and all(isinstance(x,str) and x for x in [agent,runner]),'missing_private_context'

class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self,*args,**kwargs):return None
opener=build_opener(NoRedirect())
def control(path,payload=None):
    data=None if payload is None else json.dumps(payload).encode('utf-8')
    req=Request(base+'/api/v1/'+path,data=data,headers={'Authorization':'Bearer '+agent,'Content-Type':'application/json'},method='GET' if payload is None else 'POST')
    with opener.open(req,timeout=15) as response:
        raw=response.read(2*1024*1024+1)
        assert len(raw)<=2*1024*1024,'control_response_limit'
        return json.loads(raw)
def real_task_ids():
    body=control('agent/tools/invoke',{'project_id':project,'tool':'project.tasks','arguments':{},'idempotency_key':'native-live-read-'+uuid.uuid4().hex})
    assert isinstance(body.get('items'),list),'invalid_real_task_response'
    ids=sorted(x['id'] for x in body['items'])
    assert len(ids)==len(set(ids))==3,'expected_three_real_task_ids'
    return ids

catalog=control('agent/tools?'+urlencode({'project_id':project}))
catalog_by_name={x['name']:x for x in catalog['items']}
assert set(catalog_by_name)=={'project.overview','project.tasks','project.search'},'unexpected_readonly_catalog'
task_description=catalog_by_name['project.tasks']['description']
expected_ids=real_task_ids()
state={'case':None,'calls':[],'errors':[],'verified_ids':[],'selected_alias':None}
lock=threading.Lock()

class Fixture(BaseHTTPRequestHandler):
    def log_message(self,*args):pass
    def do_POST(self):
        try:
            assert self.path=='/chat/completions','unexpected_fixture_path'
            assert not self.headers.get('Authorization'),'provider_authorization_must_be_absent'
            size=int(self.headers.get('Content-Length','0'))
            assert 0<size<=2*1024*1024,'fixture_request_limit'
            body=json.loads(self.rfile.read(size))
            tools=body.get('tools')
            assert isinstance(tools,list) and len(tools)==3,'unexpected_provider_catalog_size'
            names=[]
            for item in tools:
                assert item.get('type')=='function','unexpected_tool_type'
                function=item['function']
                assert isinstance(function.get('name'),str) and re.fullmatch(r'[A-Za-z0-9_-]{1,64}',function['name']),'illegal_provider_function_name'
                assert set(function).issubset({'name','description','parameters'}),'private_provider_function_field'
                names.append(function['name'])
            assert len(set(names))==3,'duplicate_provider_alias'
            selected=[x['function']['name'] for x in tools if x['function'].get('description')==task_description]
            assert len(selected)==1,'tasks_description_must_select_one_alias'
            summary={'path':self.path,'case':state['case'],'model_label':body.get('model'),'provider_authorization_present':False,'function_names':names,'function_name_contract_valid':True,'selected_canonical':'project.tasks','selected_alias':selected[0],'message_roles':[m.get('role') for m in body.get('messages',[])]}
            with lock:
                ordinal=len(state['calls'])+1
                state['calls'].append(summary)
                state['selected_alias']=selected[0]
            if ordinal==1:
                response={'choices':[{'message':{'role':'assistant','content':None,'tool_calls':[{'id':'fixture_read_tasks','type':'function','function':{'name':selected[0],'arguments':'{}'}}]},'finish_reason':'tool_calls'}]}
            elif ordinal==2:
                assistants=[m for m in body['messages'] if m.get('role')=='assistant' and m.get('tool_calls')]
                assert assistants and assistants[-1]['tool_calls'][0]['function']['name']==selected[0],'assistant_history_must_keep_provider_alias'
                outputs=[m for m in body['messages'] if m.get('role')=='tool' and m.get('tool_call_id')=='fixture_read_tasks']
                assert len(outputs)==1,'missing_actual_tool_result'
                real=json.loads(outputs[0]['content'])
                observed=sorted(x['id'] for x in real['items'])
                assert observed==expected_ids,'actual_control_task_ids_differ'
                with lock:
                    state['verified_ids']=observed
                    summary['actual_task_ids']=observed
                    summary['assistant_history_alias_valid']=True
                response={'choices':[{'message':{'role':'assistant','content':'Local transport fixture verified real authorized task IDs: '+', '.join(observed)+'. No model was invoked.'},'finish_reason':'stop'}]}
            else:
                raise AssertionError('unexpected_extra_model_request')
            encoded=json.dumps(response).encode('utf-8')
            self.send_response(200);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(encoded)));self.end_headers();self.wfile.write(encoded)
        except Exception as error:
            # Only static labels above are retained, never request/header content.
            label=str(error) if isinstance(error,AssertionError) else 'fixture_request_failed'
            with lock:state['errors'].append(label)
            encoded=b'{"error":"local_fixture_validation_failed"}'
            self.send_response(400);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(encoded)));self.end_headers();self.wfile.write(encoded)

server=ThreadingHTTPServer(('127.0.0.1',0),Fixture)
server.daemon_threads=True
thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
fixture_url='http://127.0.0.1:'+str(server.server_port)
node=shutil.which('node')
assert node,'node_unavailable'
cli=ROOT/'packages/agent-runtime/dist/src/cli.js'
result={'stage':'T-P4-004','recorded_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'scope':'Actual compiled native CLI + actual control service authentication and readonly project.tasks. Model endpoint is a loopback deterministic HTTP fixture; no model/provider account/key/billed request.','control_base':base,'project_id':project,'expected_real_task_ids':expected_ids,'readonly_catalog_names':sorted(catalog_by_name),'model_fixture':{'base':fixture_url,'label':'fixture-transport-only','provider_api_key_used':False},'cases':[],'source_build':[{'path':p.relative_to(ROOT).as_posix(),'sha256':hashlib.sha256(p.read_bytes()).hexdigest()} for p in [cli,cli.parent/'index.js']]}
raw_events=[]
decoy='unused-runner-decoy-'+uuid.uuid4().hex
foreign='0'*32 if project!='0'*32 else 'f'*32
failure=None
try:
    for label,credential,selected_project,success in [('agent_success',agent,project,True),('runner_credential_rejected',runner,project,False),('foreign_project_scope_rejected',agent,foreign,False)]:
        with lock:state.update(case=label,calls=[],errors=[],verified_ids=[],selected_alias=None)
        env={k:v for k,v in os.environ.items() if not k.startswith('SUMA_')}
        env.update(SUMA_MODEL='fixture-transport-only',SUMA_MODEL_BASE_URL=fixture_url,SUMA_CONTROL_URL=base,SUMA_PROJECT_ID=selected_project,SUMA_AGENT_TOKEN=credential,SUMA_RUNNER_TOKEN=decoy)
        request={'goal':'Read the current authorized project tasks using the supplied project.tasks description and return exact task IDs. Local model transport fixture only.','request_id':'native-live-'+label,'max_steps':3,'max_tokens':128}
        completed=subprocess.run([node,str(cli)],input=json.dumps(request)+'\n',env=env,cwd=ROOT,capture_output=True,text=True,encoding='utf-8',timeout=45,creationflags=getattr(subprocess,'CREATE_NO_WINDOW',0))
        for secret in [agent,runner,decoy]:
            assert secret not in completed.stdout and secret not in completed.stderr,'credential_value_in_cli_output'
        events=[json.loads(line) for line in completed.stdout.splitlines() if line.strip()]
        for event in events:raw_events.append({'case':label,'event':event})
        with lock:
            calls=list(state['calls']);errors=list(state['errors']);verified=list(state['verified_ids'])
        case={'case':label,'node_exit':completed.returncode,'model_fixture_calls':len(calls),'events_count':len(events),'event_types':[x['type'] for x in events],'provider_request_shape':calls,'fixture_errors':errors,'cli_stderr_empty':not completed.stderr,'runner_env_decoy_ignored':True,'task_ids_after':real_task_ids()}
        result['cases'].append(case)
        assert not errors and case['task_ids_after']==expected_ids,'fixture_or_task_identity_changed'
        if success:
            assert completed.returncode==0 and events[-1]['type']=='completed','native_agent_success_failed'
            assert len(calls)==2 and verified==expected_ids,'native_agent_did_not_read_actual_task_ids'
            assert [x.get('name') for x in events if x['type']=='tool_started']==['project.tasks'],'canonical_tool_event_missing'
            assert all(task_id in events[-1]['output'] for task_id in expected_ids),'completed_output_missing_actual_ids'
            case['actual_task_ids_verified']=verified
        else:
            assert completed.returncode!=0 and len(calls)==0,'unauthorized_request_reached_model'
            assert events and events[-1].get('type')=='error' and events[-1].get('code')=='upstream_rejected' and 'HTTP 403' in events[-1].get('message',''),'actual_platform_did_not_reject_with_403'
            assert not any(x['type'] in ('started','completed','tool_started') for x in events),'unauthorized_execution_fact_emitted'
            case['actual_control_rejection']=403
            if label.startswith('foreign'):case['different_project_parameter_only_no_foreign_project_created']=True
        case['passed']=True
    result['passed']=True
except Exception as error:
    failure=str(error) if isinstance(error,AssertionError) else type(error).__name__
    result['passed']=False;result['failure']=failure
finally:
    server.shutdown();server.server_close();thread.join(timeout=5)
    result['fixture_server_closed']=not thread.is_alive()
    result['control_or_runner_processes_modified']=False
    result['credentials_recorded']=False
    result['versions_or_statuses_asserted']=False
    result['model_usage']='Not reported by the local fixture; raw usage_reported:false is not measured token usage.'
    report=json.dumps(result,indent=2,ensure_ascii=False)+'\n'
    lines=''.join(json.dumps(x,ensure_ascii=False)+'\n' for x in raw_events)
    for secret in [agent,runner,decoy]:assert secret not in report and secret not in lines,'credential_value_in_evidence'
    (OUT/'native-live-scope.json').write_text(report,encoding='utf-8')
    (OUT/'native-live-scope.stdout.jsonl').write_text(lines,encoding='utf-8')
    print(json.dumps({'passed':result['passed'],'cases':[{'case':x['case'],'node_exit':x['node_exit'],'model_calls':x['model_fixture_calls'],'passed':x.get('passed',False)} for x in result['cases']],'task_ids':expected_ids,'fixture_server_closed':result['fixture_server_closed'],'failure':failure},ensure_ascii=True))
if failure:raise SystemExit(1)
