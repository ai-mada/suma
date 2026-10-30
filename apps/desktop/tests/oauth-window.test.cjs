'use strict';
const {test}=require('node:test');
const assert=require('node:assert/strict');
const {EventEmitter}=require('node:events');
const {createWorkBuddyOAuthController}=require('../src/oauth-window.cjs');
const origin='http://127.0.0.1:43959',callback=origin+'/api/v1/workbuddy-oauth/callback';
const authID='a'.repeat(32),connectionID='b'.repeat(32),state=authID+'.'+Buffer.alloc(32,7).toString('base64url');
const request={project_id:'project-a',authorization_id:authID,user_id:'owner-a',tenant_id:'tenant-a'};
const expires=Date.now()+240000;
function authURL(){return 'https://www.workbuddy.cn/openapi/v2/authorize?'+new URLSearchParams({response_type:'code',client_id:'client-a',redirect_uri:callback,state,scope:'user.task.readable user.task.invokable'});}
function dto(){return {id:authID,version:1,state:'pending',expires_at:new Date(expires).toISOString(),authorization_url:authURL(),connection:{id:connectionID,project_id:request.project_id,owner_id:request.user_id,client_id:'client-a',state:'authorizing'}};}
function identityResponse(){return new Response(JSON.stringify({user:{id:request.user_id},organization:{id:request.tenant_id}}),{headers:{'Content-Type':'application/json'}});}
function setAuthorizationFetch(h,fn){h.session.fetch=(url,init)=>url.endsWith('/auth/session')?Promise.resolve(identityResponse()):fn(url,init);}
function harness(t,options={}){
 const calls=[],windows=[];let response=dto();
 const session=new EventEmitter();session.fetch=async(url,init)=>{calls.push({url,init});return url.endsWith('/auth/session')?identityResponse():new Response(JSON.stringify(response),{headers:{'Content-Type':'application/json'}});};
 class Contents extends EventEmitter{constructor(){super();this.session=session;this.mainFrame={url:origin+'/?draft=keep'};this.dead=false;}getURL(){return this.mainFrame.url;}isDestroyed(){return this.dead;}setWindowOpenHandler(fn){this.openHandler=fn;}}
 class Window extends EventEmitter{constructor(opts){super();this.options=opts;this.webContents=new Contents();this.dead=false;this.focusCount=0;this.loaded=[];if(opts)windows.push(this);}isDestroyed(){return this.dead;}focus(){this.focusCount++;}destroy(){if(!this.dead){this.dead=true;this.webContents.dead=true;this.emit('closed');this.webContents.emit('destroyed');}}loadURL(url){this.loaded.push(url);return Promise.resolve();}}
 const parent=new Window(),event={sender:parent.webContents,senderFrame:parent.webContents.mainFrame};
 const controller=createWorkBuddyOAuthController({BrowserWindow:Window,parent,origin,clientID:'client-a',allowedScopes:['user.task.readable','user.task.invokable'],...options});
 t.after(()=>controller.dispose());
 return {controller,calls,windows,parent,event,session,setResponse(value){response=value;}};
}
function nav(window,url,eventName='will-navigate',main=true){const e={url,isMainFrame:main,prevented:false,preventDefault(){this.prevented=true;}};window.webContents.emit(eventName,e,url,false,main);return e;}
function commit(window,url){window.webContents.mainFrame.url=url;window.webContents.emit('did-navigate',{},url,200,'OK');}
function safeReturn(){return origin+'/?'+new URLSearchParams({project:request.project_id,view:'settings',sub:'tools',panel:'workbuddy:'+connectionID});}

test('reads exact authorization with original session and opens isolated window; callback preserves parent draft',async t=>{
 const h=harness(t);assert.deepEqual(await h.controller.open(h.event,request),{status:'opened'});
 assert.equal(h.calls.length,2);assert.equal(h.calls[0].url,origin+'/api/v1/auth/session');assert.equal(h.calls[1].url,origin+'/api/v1/workbuddy-authorizations/'+authID+'?project_id=project-a');
 assert.equal(h.calls[0].init.credentials,'include');assert.equal(h.calls[0].init.redirect,'error');assert.equal(h.calls[0].init.headers['X-Suma-Expected-User'],request.user_id);assert.equal(h.calls[0].init.headers['X-Suma-Expected-Organization'],request.tenant_id);
 const child=h.windows[0],prefs=child.options.webPreferences;assert.equal(child.options.parent,h.parent);assert.equal(prefs.session,h.session);assert.equal(prefs.preload,undefined);assert.equal(prefs.contextIsolation,true);assert.equal(prefs.sandbox,true);assert.equal(prefs.nodeIntegration,false);assert.equal(prefs.webSecurity,true);assert.equal(prefs.allowRunningInsecureContent,false);
 assert.equal(child.loaded[0],authURL());assert.deepEqual(child.webContents.openHandler({url:origin}),{action:'deny'});
 const cb=callback+'?'+new URLSearchParams({state,code:'private-code'});assert.equal(nav(child,cb).prevented,false);commit(child,cb);
 child.webContents.emit('did-navigate-in-page',{},callback,true);assert.equal(child.dead,false);
 assert.equal(nav(child,safeReturn()).prevented,true);assert.equal(child.dead,true);assert.equal(h.parent.focusCount,1);assert.equal(h.parent.webContents.getURL(),origin+'/?draft=keep');assert.equal(h.session.listenerCount('will-download'),0);
});

test('rejects forged IPC identity, frame and request before fetching',async t=>{
 const h=harness(t);
 for(const [event,body] of [[{...h.event,sender:{}},request],[{...h.event,senderFrame:{url:origin}},request],[h.event,{...request,url:authURL()}],[h.event,{...request,project_id:'../other'}],[h.event,{...request,tenant_id:''}]]){assert.equal((await h.controller.open(event,body)).status,'failed');}
 h.parent.webContents.mainFrame.url='https://evil.example/';assert.equal((await h.controller.open(h.event,request)).status,'failed');assert.equal(h.calls.length,0);assert.equal(h.windows.length,0);
});

test('rejects mismatched or expired authority and malicious authorization URLs',async t=>{
 const mutations=[d=>d.id='c'.repeat(32),d=>d.connection.owner_id='other',d=>d.connection.project_id='other',d=>d.connection.client_id='other',d=>d.state='exchanging',d=>d.connection.state='connected',d=>d.expires_at='2000-01-01T00:00:00Z',d=>d.authorization_url+='&scope=user.task.readable',d=>d.authorization_url+='&extra=secret',d=>d.authorization_url=d.authorization_url.replace('https://www.workbuddy.cn/','https://www.workbuddy.cn:443/'),d=>d.authorization_url=d.authorization_url.replace('client-a','other'),d=>d.authorization_url=d.authorization_url.replace('user.task.invokable','user.localassistant.readable'),d=>d.authorization_url=d.authorization_url.replace(state,authID+'.'+'A'.repeat(42)+'B'),d=>d.authorization_url=d.authorization_url.replace('www.workbuddy.cn','www.workbuddy.cn.evil.example')];
 for(const mutate of mutations){const h=harness(t),d=dto();mutate(d);h.setResponse(d);assert.equal((await h.controller.open(h.event,request)).status,'failed');assert.equal(h.windows.length,0);}
});

test('one pending open; exact replay focuses; different authorization never replaces state',async t=>{
 const h=harness(t);let resolve;setAuthorizationFetch(h,()=>{h.calls.push(1);return new Promise(r=>resolve=r);});
 const first=h.controller.open(h.event,request),same=h.controller.open(h.event,{...request});assert.equal((await h.controller.open(h.event,{...request,authorization_id:'c'.repeat(32)})).status,'failed');
 await new Promise(setImmediate);resolve(new Response(JSON.stringify(dto()),{headers:{'Content-Type':'application/json'}}));assert.equal((await first).status,'opened');assert.equal((await same).status,'opened');assert.equal(h.calls.length,1);assert.equal(h.windows.length,1);
 assert.equal((await h.controller.open(h.event,request)).status,'opened');assert.equal(h.windows[0].focusCount,1);
});

test('rechecks invoking frame after authorization fetch and never opens for late results',async t=>{
 const h=harness(t);let resolve;setAuthorizationFetch(h,()=>new Promise(r=>resolve=r));const pending=h.controller.open(h.event,request);await new Promise(setImmediate);h.parent.webContents.mainFrame={url:origin};resolve(new Response(JSON.stringify(dto()),{headers:{'Content-Type':'application/json'}}));assert.equal((await pending).status,'failed');assert.equal(h.windows.length,0);
});

test('callback and safe return cannot escape original state, project or connection',async t=>{
 const attempts=[safeReturn(),callback+'?state=wrong&code=secret',callback+'?'+new URLSearchParams({state,code:'secret',error:'access_denied'}),'https://login.evil.example/','file:///tmp/oauth','https://www.workbuddy.cn:443/next'];
 for(const url of attempts){const h=harness(t);await h.controller.open(h.event,request);assert.equal(nav(h.windows[0],url,'will-redirect').prevented,true);assert.equal(h.windows[0].dead,true);assert.equal(h.parent.focusCount,0);}
 for(const changed of ['project','panel']){const h=harness(t);await h.controller.open(h.event,request);commit(h.windows[0],callback+'?'+new URLSearchParams({state,code:'secret'}));const u=new URL(safeReturn());u.searchParams.set(changed,'other');assert.equal(nav(h.windows[0],u.href).prevented,true);assert.equal(h.parent.focusCount,0);}
});

test('subframes cannot invoke callback; download guard only affects owned child',async t=>{
 const h=harness(t);await h.controller.open(h.event,request);const child=h.windows[0];let prevented=0;
 h.session.emit('will-download',{preventDefault(){prevented++;}},{},h.parent.webContents);assert.equal(prevented,0);
 h.session.emit('will-download',{preventDefault(){prevented++;}},{},child.webContents);assert.equal(prevented,1);
 assert.equal(nav(child,callback+'?'+new URLSearchParams({state,code:'secret'}),'will-frame-navigate',false).prevented,true);assert.equal(child.dead,true);
});

test('timeout, user close, disposal and parent teardown release owned listeners and windows',async t=>{
 t.mock.timers.enable({apis:['setTimeout']});
 const h=harness(t);await h.controller.open(h.event,request);t.mock.timers.tick(300001);assert.equal(h.windows[0].dead,true);assert.equal(h.session.listenerCount('will-download'),0);
 await h.controller.open(h.event,request);h.windows[1].destroy();assert.equal(h.session.listenerCount('will-download'),0);
 await h.controller.open(h.event,request);h.parent.destroy();assert.equal(h.windows[2].dead,true);assert.equal((await h.controller.open(h.event,request)).status,'failed');assert.equal(h.session.listenerCount('will-download'),0);
});

test('fetch and load failures return safe codes without exposing callback secrets',async t=>{
 const h=harness(t);h.session.fetch=async()=>{throw Error(authURL()+'private-code');};const result=await h.controller.open(h.event,request);assert.equal(result.status,'failed');assert.match(result.code,/^[a-z_]+$/);assert.equal(JSON.stringify(result).includes(state),false);assert.equal(h.windows.length,0);
 const other=harness(t);other.setResponse({...dto(),authorization_url:'javascript:alert(1)'});assert.equal((await other.controller.open(other.event,request)).status,'failed');
});

test('malformed connection identity and noncanonical authorize path are rejected',async t=>{
 for(const mutate of [d=>delete d.connection.id,d=>d.connection.id=null,d=>d.authorization_url=d.authorization_url.replace('/openapi/','/ignored/../openapi/')]){
  const h=harness(t),d=dto();mutate(d);h.setResponse(d);assert.equal((await h.controller.open(h.event,request)).status,'failed');assert.equal(h.windows.length,0);
 }
});

test('access_denied callback uses the same bridge; oversized or redirected API bodies never open windows',async t=>{
 const h=harness(t);await h.controller.open(h.event,request);const child=h.windows[0],cb=callback+'?'+new URLSearchParams({state,error:'access_denied'});
 assert.equal(nav(child,cb).prevented,false);commit(child,cb);assert.equal(nav(child,safeReturn()).prevented,true);assert.equal(child.dead,true);assert.equal(h.parent.focusCount,1);
 for(const response of [new Response('x'.repeat(65537),{headers:{'Content-Type':'application/json'}}),new Response('{}',{status:302,headers:{Location:'https://evil.example'}})]){
  const other=harness(t);setAuthorizationFetch(other,async()=>response);assert.equal((await other.controller.open(other.event,request)).status,'failed');assert.equal(other.windows.length,0);
 }
});

test('load failure, disposal during fetch, and elapsed opening budget cannot leak owned resources',async t=>{
 const h=harness(t);let resolve;setAuthorizationFetch(h,()=>new Promise(r=>resolve=r));const pending=h.controller.open(h.event,request);await new Promise(setImmediate);h.controller.dispose();resolve(new Response(JSON.stringify(dto()),{headers:{'Content-Type':'application/json'}}));assert.equal((await pending).status,'failed');assert.equal(h.windows.length,0);
 let time=Date.now();const late=harness(t,{now:()=>time});setAuthorizationFetch(late,async()=>{time+=300001;const d=dto();d.expires_at=new Date(time+60000).toISOString();return new Response(JSON.stringify(d),{headers:{'Content-Type':'application/json'}});});assert.equal((await late.controller.open(late.event,request)).status,'failed');assert.equal(late.windows.length,0);
 const broken=harness(t);let finish;setAuthorizationFetch(broken,()=>new Promise(r=>finish=r));const opening=broken.controller.open(broken.event,request);await new Promise(setImmediate);
 const original=broken.parent.constructor.prototype.loadURL;broken.parent.constructor.prototype.loadURL=async()=>{throw Error('private-code '+state);};finish(new Response(JSON.stringify(dto()),{headers:{'Content-Type':'application/json'}}));
 const result=await opening;broken.parent.constructor.prototype.loadURL=original;assert.deepEqual(result,{status:'failed',code:'oauth_open_failed'});assert.equal(broken.windows[0].dead,true);assert.equal(broken.session.listenerCount('will-download'),0);
});

test('current session identity precedes authority lookup; server client ID works without local operator configuration',async t=>{
 const h=harness(t,{clientID:undefined,allowedScopes:undefined});
 h.session.fetch=async(url,init)=>{h.calls.push({url,init});return new Response(JSON.stringify(url===origin+'/api/v1/auth/session'?{user:{id:request.user_id},organization:{id:request.tenant_id}}:dto()),{headers:{'Content-Type':'application/json'}});};
 assert.deepEqual(await h.controller.open(h.event,request),{status:'opened'});
 assert.deepEqual(h.calls.map(call=>call.url),[origin+'/api/v1/auth/session',origin+'/api/v1/workbuddy-authorizations/'+authID+'?project_id=project-a']);
 for(const call of h.calls){assert.equal(call.init.credentials,'include');assert.equal(call.init.redirect,'error');assert.equal(call.init.cache,'no-store');assert.equal(call.init.headers['X-Suma-Expected-User'],request.user_id);assert.equal(call.init.headers['X-Suma-Expected-Organization'],request.tenant_id);}
 const mismatched=harness(t,{clientID:undefined,allowedScopes:undefined});mismatched.session.fetch=async(url)=>{const d=dto();d.connection.client_id='server-other';return new Response(JSON.stringify(url.endsWith('/auth/session')?{user:{id:request.user_id},organization:{id:request.tenant_id}}:d),{headers:{'Content-Type':'application/json'}});};assert.equal((await mismatched.controller.open(mismatched.event,request)).status,'failed');assert.equal(mismatched.windows.length,0);
});

test('wrong current tenant, user or changed frame stops before authorization lookup',async t=>{
 for(const change of ['tenant','user','frame']){
  const h=harness(t);h.session.fetch=async(url,init)=>{h.calls.push({url,init});const identity={user:{id:request.user_id},organization:{id:request.tenant_id}};if(change==='tenant')identity.organization.id='other';if(change==='user')identity.user.id='other';if(change==='frame')h.parent.webContents.mainFrame={url:origin};return new Response(JSON.stringify(url.endsWith('/auth/session')?identity:dto()),{headers:{'Content-Type':'application/json'}});};
  assert.equal((await h.controller.open(h.event,request)).status,'failed');assert.equal(h.calls.length,1);assert.equal(h.calls[0].url,origin+'/api/v1/auth/session');assert.equal(h.windows.length,0);
 }
});

test('denied callback accepts bounded description without reflecting it and rejects ambiguous fields',async t=>{
 const description='User declined private consent';
 const h=harness(t);await h.controller.open(h.event,request);const child=h.windows[0];
 const cb=callback+'?'+new URLSearchParams({state,error:'access_denied',error_description:description});
 assert.equal(nav(child,cb).prevented,false);commit(child,cb);child.webContents.emit('did-navigate-in-page',{},callback,true);
 assert.equal(nav(child,safeReturn()).prevented,true);assert.equal(child.dead,true);assert.equal(h.parent.focusCount,1);
 assert.equal(JSON.stringify(h.calls).includes(description),false);
 for(const suffix of ['&error_description=again','&unknown=value']){
  const other=harness(t);await other.controller.open(other.event,request);assert.equal(nav(other.windows[0],cb+suffix).prevented,true);assert.equal(other.parent.focusCount,0);
 }
 for(const text of ['x'.repeat(2049),'unsafe\u0000text','unsafe\ntext']){
  const other=harness(t);await other.controller.open(other.event,request);assert.equal(nav(other.windows[0],callback+'?'+new URLSearchParams({state,error:'access_denied',error_description:text})).prevented,true);
 }
 const success=harness(t);await success.controller.open(success.event,request);assert.equal(nav(success.windows[0],callback+'?'+new URLSearchParams({state,code:'secret',error_description:description})).prevented,true);
});
