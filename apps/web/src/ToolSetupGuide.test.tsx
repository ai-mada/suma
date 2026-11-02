// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {setLocale} from './i18n';
import {ToolSetupGuide} from './ToolSetupGuide';

let root:Root|undefined,host:HTMLDivElement,clipboard:PropertyDescriptor|undefined;
beforeEach(async()=>{await setLocale('zh-CN',{persist:false});host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);clipboard=Object.getOwnPropertyDescriptor(navigator,'clipboard');});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();if(clipboard)Object.defineProperty(navigator,'clipboard',clipboard);else Reflect.deleteProperty(navigator,'clipboard');vi.unstubAllGlobals();vi.restoreAllMocks();});
async function change(element:HTMLInputElement|HTMLSelectElement,value:string){const proto=element instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(proto,'value')!.set!.call(element,value);element.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});}

it('T203 carries nine precise preparation choices into the same target without overwriting its manual shell and paths on ordinary renders',async()=>{
 const fetch=vi.fn();vi.stubGlobal('fetch',fetch);const choices=[['codex','codex-jsonl'],['codebuddy','codebuddy-cli'],['harness','dsh-multica'],['claude','claude-code-cli'],['qoder','qoder-acp'],['opencode','opencode-sdk'],['cursor','cursor-cli'],['claude-sdk','claude-code-sdk'],['codex-app-server','codex-app-server']] as const;
 let nonce=0;root=createRoot(host);const field=(name:string)=>host.querySelector<HTMLInputElement|HTMLSelectElement>('[name="remote-runner-'+name+'"]')!;
 for(const [preset,localTool] of choices){const request={preset,nonce:++nonce};await act(async()=>root!.render(<ToolSetupGuide expanded expectedWorkerID="original-node" presetRequest={request}/>));
  expect(host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!.value).toBe(preset);expect(field('localTool').value).toBe(localTool);expect(field('worker').value).toBe('original-node');
  if(nonce===1){await change(field('shell'),'bash');await change(field('commands'),'/srv/manual.commands.json');await change(field('repository'),'/srv/original');await change(field('baseCommit'),'a'.repeat(40));}
  expect(field('commands').value).toBe('/srv/manual.commands.json');expect(field('repository').value).toBe('/srv/original');expect(field('shell').value).toBe('bash');
  const prepare=host.querySelector('[data-remote-runner-command="prepare"]');if(nonce<=4)expect(prepare!.textContent).toContain("--local-tool '"+localTool+"'");else expect(prepare).toBeNull();
  await change(field('commands'),'/srv/manual.edited.json');await act(async()=>root!.render(<ToolSetupGuide expanded expectedWorkerID="original-node" presetRequest={{...request}}/>));expect(field('commands').value).toBe('/srv/manual.edited.json');await change(field('commands'),'/srv/manual.commands.json');
 }
 expect(host.querySelector('[data-codex-app-server-setup-guide]')).not.toBeNull();expect(fetch).not.toHaveBeenCalled();
});

it('T192 uses the independent SDK guide and hands only its public draft to the original Runner flow',async()=>{
 const fetch=vi.fn();vi.stubGlobal('fetch',fetch);
 await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide expanded presetRequest={{preset:'claude-sdk',nonce:192}}/>);});
 expect(host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!.value).toBe('claude-sdk');
 const values={shell:'powershell',node:'C:/Program Files/nodejs/node.exe',host:'D:/suma-sdk/suma-claude.bundle.mjs',nativeCLI:'D:/tools/claude.exe',model:'claude-sonnet-5',credentialEnvRef:'ANTHROPIC_API_KEY',commands:'D:/runner/commands.sdk.json'};
 for(const [name,value] of Object.entries(values))await change(host.querySelector<HTMLInputElement|HTMLSelectElement>('[data-claude-sdk-setup-field="'+name+'"]')!,value);
 const use=host.querySelector<HTMLButtonElement>('[data-claude-sdk-setup-use]')!;expect(use.disabled).toBe(false);await act(async()=>use.click());
 expect(host.querySelector<HTMLInputElement>('[name="remote-runner-commands"]')!.value).toBe(values.commands);expect(host.querySelector<HTMLSelectElement>('[name="remote-runner-localTool"]')!.value).toBe('claude-code-sdk');
 expect(host.querySelector('[data-remote-runner-command="prepare"]')).toBeNull();
 const configure=host.querySelector<HTMLButtonElement>('[data-remote-runner-configure-claude-sdk]')!;await act(async()=>configure.click());
 expect(host.querySelector<HTMLInputElement>('[data-claude-sdk-setup-field="commands"]')!.value).toBe(values.commands);expect(fetch).not.toHaveBeenCalled();
 await change(host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!,'claude');expect(host.querySelector('[data-claude-sdk-setup-guide]')).toBeNull();expect(host.textContent).toContain('claude-jsonl-v1');
});

it('T191 carries an explicit Cursor configuration draft into the original remote Runner guide without writing or enabling',async()=>{
 const fetch=vi.fn(async(_input:RequestInfo|URL,_init?:RequestInit)=>Response.json({items:[]}));vi.stubGlobal('fetch',fetch);
 await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide expanded presetRequest={{preset:'cursor',nonce:191}}/>);});
 const select=host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!;expect(select.value).toBe('cursor');
 const values={shell:'powershell',packageRoot:'D:/tools/Cursor Agent',reviewedVersion:'Cursor exact 191',model:'reviewed-model',credentialEnvRef:'LOCAL_CURSOR_CREDENTIAL',commands:'D:/suma/commands.cursor.json'};
 for(const [name,value] of Object.entries(values))await change(host.querySelector<HTMLInputElement|HTMLSelectElement>('[data-cursor-setup-field="'+name+'"]')!,value);
 const use=host.querySelector<HTMLButtonElement>('[data-cursor-setup-use]')!;expect(use.disabled).toBe(false);await act(async()=>use.click());
 expect(host.querySelector<HTMLInputElement>('[name="remote-runner-commands"]')!.value).toBe(values.commands);
 expect(host.querySelector<HTMLSelectElement>('[name="remote-runner-shell"]')!.value).toBe(values.shell);
 expect(host.querySelector<HTMLSelectElement>('[name="remote-runner-localTool"]')!.value).toBe('cursor-cli');
 expect(host.querySelector('[data-remote-runner-command="prepare"]')).toBeNull();expect(fetch).not.toHaveBeenCalled();
 await act(async()=>root!.render(<ToolSetupGuide expanded project="another-project" presetRequest={{preset:'cursor',nonce:192}}/>));
 expect(host.querySelector<HTMLInputElement>('[data-cursor-setup-field="commands"]')!.value).toBe('');
 expect(host.querySelector<HTMLInputElement>('[name="remote-runner-commands"]')!.value).toBe('');expect(fetch.mock.calls.every(([,init])=>!init?.method||init.method==='GET')).toBe(true);
});

it.each([['extractor','知识提炼模型','knowledge_extraction'],['diagnosis','失败分诊模型','failure_diagnosis'],['operations','智能操作规划模型','intelligent_operations']] as const)('configures the %s model with explicit valid inputs and no planning or control credentials',async(_preset,title,capability)=>{
 const writeText=vi.fn(async(_text:string)=>{});Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText}});
 await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide/>);});
 const details=host.querySelector('details')!;await act(async()=>{details.open=true;details.dispatchEvent(new Event('toggle'));});
 const option=[...host.querySelectorAll('option')].find(value=>value.textContent===title);expect(option).toBeDefined();await change(option!.parentElement as HTMLSelectElement,option!.value);
 const copy=()=>host.querySelector<HTMLButtonElement>('.tool-cli-setup-actions button')!,entry=host.querySelector<HTMLInputElement>('.tool-cli-setup-entry input')!;
 expect(copy().disabled).toBe(true);await change(entry,'relative.mjs');expect(copy().disabled).toBe(true);await change(entry,'C:/tools/suma/suma-agent.mjs');expect(copy().disabled).toBe(true);
 const model=host.querySelector<HTMLInputElement>('.form-grid input:not([type=url])')!,endpoint=host.querySelector<HTMLInputElement>('.form-grid input[type=url]')!;
 await change(model,'configured-extraction-model');await change(endpoint,'https://provider.example.test/v1?token=do-not-use');expect(copy().disabled).toBe(true);await change(endpoint,'https://provider.example.test/v1');expect(copy().disabled).toBe(false);
 const literal=host.querySelector('pre')!.textContent!,configuration=JSON.parse(literal),command=Object.values(configuration)[0] as Record<string,unknown>;
 expect(command).toMatchObject({adapter:'suma',input:'agent-jsonl',read_only:true,[capability]:true,executable:'node',args:['C:/tools/suma/suma-agent.mjs'],env:{SUMA_MODEL:'configured-extraction-model',SUMA_MODEL_BASE_URL:'https://provider.example.test/v1'},env_from:['SUMA_MODEL_API_KEY']});expect(command).not.toHaveProperty('planning');expect(literal).not.toContain('SUMA_CONTROL_URL');expect(literal).not.toContain('SUMA_AGENT_TOKEN');expect(literal).not.toContain('SUMA_PROJECT_ID');
 await act(async()=>copy().click());expect(writeText).toHaveBeenCalledExactlyOnceWith(literal);
});

it('T118 native preset requires shared platform model binding and explicit local repair opt-in',async()=>{
 await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide/>);});
 const tool=host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!;expect(JSON.parse(host.querySelector('pre')!.textContent!)['codex-develop']).not.toHaveProperty('native_workspace');
 const option=[...tool.options].find(value=>value.value==='native-coder');expect(option).toBeDefined();await change(tool,'native-coder');
 const copy=()=>host.querySelector<HTMLButtonElement>('.tool-cli-setup-actions button')!;expect(copy().disabled).toBe(true);
 await change(host.querySelector<HTMLInputElement>('.tool-cli-setup-entry input')!,'D:/tools/suma/suma-agent.bundle.mjs');await change(host.querySelector<HTMLInputElement>('[name=native_runtime_manifest]')!,'D:/tools/suma/suma-agent.manifest.json');await change(host.querySelector<HTMLInputElement>('[name=native_check_ids]')!,'formal-check');expect(copy().disabled).toBe(false);
 const command=()=>Object.values(JSON.parse(host.querySelector('pre')!.textContent!))[0] as any;
 expect(command().model_execution_protocol).toBe('suma-model-execution-v1');expect(command()).not.toHaveProperty('env_from');expect(command()).not.toHaveProperty('env');expect(host.textContent).toContain('model_deployments');expect(command().native_workspace).toEqual({checks:['formal-check'],runtime_manifest:'D:/tools/suma/suma-agent.manifest.json'});const optIn=host.querySelector<HTMLInputElement>('[name=native_conflict_repair]')!;expect(optIn.checked).toBe(false);await act(async()=>optIn.click());expect(command().native_workspace.conflict_repair).toBe(true);expect(host.textContent).toContain('浏览器不会修改 Runner 策略');await change(tool,'codex');expect(JSON.parse(host.querySelector('pre')!.textContent!)['codex-develop']).not.toHaveProperty('native_workspace');
});

it('configures real Node JUnit reporting with a runner-assigned path and no model credentials',async()=>{
 await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide/>);});await change(host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!,'junit');
 const configuration=JSON.parse(host.querySelector('pre')!.textContent!),command=configuration['junit-tests'];expect(command).toMatchObject({adapter:'command',protocol:'text-v1',executable:'node',input:'none',read_only:false,test_evidence:true,test_report:true,args:['--test','--test-reporter=junit','--test-reporter-destination={{test_report_path}}']});expect(command).not.toHaveProperty('env_from');expect(command).not.toHaveProperty('env');expect(host.querySelector('.tool-cli-setup-entry')).toBeNull();expect(host.querySelector<HTMLButtonElement>('.tool-cli-setup-actions button')?.disabled).toBe(false);
});

it('T122 recommendation preset exposes only the dedicated read-only host-model protocol',async()=>{
 await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide/>);});await change(host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!,'recommendation');
 const copy=()=>host.querySelector<HTMLButtonElement>('.tool-cli-setup-actions button')!;expect(copy().disabled).toBe(true);expect(host.querySelector('.form-grid')).toBeNull();expect(host.querySelector('[name=native_runtime_manifest]')).toBeNull();
 const entry=host.querySelector<HTMLInputElement>('.tool-cli-setup-entry input')!;await change(entry,'relative.mjs');expect(copy().disabled).toBe(true);await change(entry,'D:/tools/suma/suma-agent.bundle.mjs');expect(copy().disabled).toBe(false);
 const command=JSON.parse(host.querySelector('pre')!.textContent!)['suma-dispatch-recommendation'];expect(command).toEqual({adapter:'suma',protocol:'suma-jsonl-v1',executable:'node',args:['D:/tools/suma/suma-agent.bundle.mjs'],input:'agent-jsonl',read_only:true,dispatch_recommendation:true,model_execution_protocol:'suma-model-execution-v1'});
 expect(command).not.toHaveProperty('env');expect(command).not.toHaveProperty('env_from');expect(command).not.toHaveProperty('native_workspace');expect(host.querySelector('.tool-cli-setup-command')!.textContent).not.toContain('--repository');
});
it('T118 configures runtime-only Octop ACP with shared platform model selection',async()=>{
 await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide/>);});await change(host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!,'octop');
 const copy=()=>host.querySelector<HTMLButtonElement>('[data-octop-setup-copy]')!;expect(copy()).not.toBeNull();expect(copy().disabled).toBe(true);
 const inputs={python:'C:/Python313/python.exe',bundle:'D:/tools/suma/octop-runtime.pyz',manifest:'D:/tools/suma/octop-runtime.manifest.json',checks:'formal-check'};
 for(const [name,text] of Object.entries(inputs))await change(host.querySelector<HTMLInputElement>('[name="octop-'+name+'"]')!,text);
 expect(copy().disabled).toBe(false);const config=JSON.parse(host.querySelector('[data-octop-setup-config]')!.textContent!),command=config['octop-harness-develop'];expect(command.model_execution_protocol).toBe('suma-model-execution-v1');expect(command).not.toHaveProperty('octop');expect(host.querySelector('[name=octop-provider]')).toBeNull();expect(command).not.toHaveProperty('env_from');expect(command).not.toHaveProperty('env');expect(command.args).toEqual([inputs.bundle]);expect(host.textContent).toContain('model_deployments');
 await change(host.querySelector<HTMLInputElement>('[name="octop-bundle"]')!,'relative.pyz');expect(copy().disabled).toBe(true);
});
it('T207 offers the OpenCode new-file preparation guide and copies only the explicit private-stdin command',async()=>{
 const writeText=vi.fn(async(_text:string)=>{}),fetch=vi.fn();Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText}});vi.stubGlobal('fetch',fetch);
 await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide expanded/>);});
 const select=host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!;expect([...select.options].some(option=>option.value==='opencode')).toBe(true);await change(select,'opencode');
 const copy=()=>host.querySelector<HTMLButtonElement>('[data-opencode-setup-copy]')!;expect(copy().disabled).toBe(true);
 const inputs={shell:'powershell',node:'C:/Program Files/nodejs/node.exe',manifest:'D:/suma/suma-opencode.manifest.json',executable:'D:/tools/opencode.exe',provider:'approved',model:'approved-model',endpoint:'https://models.example/v1',credential:'MODEL_PROVIDER_API_KEY',commands:'D:/suma/opencode.commands.json'};
 for(const [name,value] of Object.entries(inputs))await change(host.querySelector<HTMLInputElement>('[name="opencode-'+name+'"]')!,value);
 expect(copy().disabled).toBe(false);const text=host.querySelector('[data-opencode-setup-prepare]')!.textContent!,request=JSON.parse(host.querySelector('[data-opencode-setup-configuration]')!.textContent!);expect(Object.keys(request)).toHaveLength(9);expect(request).toMatchObject({adapter_id:'opencode-sdk',node_executable:inputs.node,native_cli_executable:inputs.executable,runtime_manifest:inputs.manifest,provider_id:inputs.provider,model:inputs.model,base_url:inputs.endpoint,api_key_env_ref:inputs.credential,reviewed_version:'1.19.4'});expect(request).not.toHaveProperty('checks');expect(request).not.toHaveProperty('bundle');expect(text).toContain('--prepare-opencode-stdin');expect(text).not.toContain('--allow-exec');
 await act(async()=>copy().click());expect(writeText).toHaveBeenCalledExactlyOnceWith(text);expect(fetch).not.toHaveBeenCalled();
 await change(host.querySelector<HTMLInputElement>('[name="opencode-credential"]')!,'secret-value');expect(copy().disabled).toBe(true);
 await change(select,'codex');await act(async()=>root!.render(<ToolSetupGuide expanded presetRequest={{preset:'opencode',nonce:177}}/>));expect(select.value).toBe('opencode');expect(host.querySelector('[data-opencode-setup-copy]')).not.toBeNull();expect(fetch).not.toHaveBeenCalled();
});
