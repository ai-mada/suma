import { tr as t, renderDiagnostic, UiError } from "./i18n";
import type {Event as WireRunEvent} from '../../../packages/contracts/src/index';
export type RunEvent=WireRunEvent;
export type RunConnectionState={state:'connecting'|'live'|'reconnecting'|'forbidden'|'invalid';message?:string;attempt?:number;diagnostic?:UiError;stopped?:boolean};
class StreamFailure extends UiError{constructor(key:string,public status=0,params?:Record<string,unknown>){super(key,params);}}
export function runConnectionMessage(state:RunConnectionState){const reason=renderDiagnostic(state.diagnostic||state.message||'');return state.stopped?t("development.events.stopped",{reason}):reason;}
// A 2 MiB accepted command can grow sixfold under Go's JSON HTML escaping.
// The frame and retained JSON budgets include room for the persisted envelope.
const maxEventBytes=16*1024*1024,encoder=new TextEncoder();

function sequence(raw:string){if(!/^\d+$/.test(raw)||!Number.isSafeInteger(Number(raw)))throw new StreamFailure("development.events.invalid_event_cursor_format",400);return Number(raw);}

// This is a line parser, not a chunk parser: UTF-8 characters, CRLF and frames
// can all cross network chunks. EOF never dispatches an incomplete final frame.
export class RunEventParser{
 private decoder=new TextDecoder('utf-8',{fatal:true});private line:string[]=[];private afterCR=false;private name='';private id='';private data:string[]=[];private size=0;
 constructor(private run:string,private accept:(event:RunEvent)=>void){}
 push(bytes:Uint8Array){let text:string;try{text=this.decoder.decode(bytes,{stream:true});}catch{throw new StreamFailure("development.events.the_event_is_not_valid_utf_8",400);}let start=0;
  if(this.afterCR&&text){this.afterCR=false;if(text[0]==='\n')start=1;}
  const newline=/\r\n|\r|\n/g;newline.lastIndex=start;let match:RegExpExecArray|null;
  while((match=newline.exec(text))){this.append(text.slice(start,match.index));
   // Reserve a possible LF for a CR at the end of a network chunk. This keeps
   // the bound conservative without delaying SSE's immediate CR dispatch.
   this.reserve(match[0]==='\r'?2:match[0].length);this.consumeLine();start=newline.lastIndex;
   if(match[0]==='\r'&&start===text.length)this.afterCR=true;
  }
  if(start<text.length)this.append(text.slice(start));
 }
 private reserve(bytes:number){this.size+=bytes;if(this.size>maxEventBytes)throw new StreamFailure("development.events.the_event_exceeds_the_16_mib_read",400);}
 private append(value:string){if(!value)return;this.reserve(encoder.encode(value).byteLength);this.line.push(value);}
 end(){this.decoder.decode();this.line=[];this.data=[];this.id='';this.name='';this.size=0;}
 private consumeLine(){const line=this.line.join('');this.line=[];
  if(!line){this.dispatch();return;}if(line.startsWith(':'))return;
  const colon=line.indexOf(':'),field=colon<0?line:line.slice(0,colon);let value=colon<0?'':line.slice(colon+1);if(value.startsWith(' '))value=value.slice(1);
  if(field==='event')this.name=value;if(field==='id')this.id=value;if(field==='data')this.data.push(value);
 }
 private dispatch(){const name=this.name,id=this.id,data=this.data.join('\n');this.name='';this.id='';this.data=[];this.size=0;
  if(name!=='run_event'&&name!=='stream_error')return;
  let value:any;try{value=JSON.parse(data);}catch{throw new StreamFailure("development.events.invalid_event_content_format",400);}
  if(name==='stream_error')throw new StreamFailure(value?.status===401?"development.events.session_expired":"development.events.live_event_permissions_or_service_expired",Number(value?.status)||500);
  const n=sequence(id);
  if(!value||value.sequence!==n||n<1||value.run_id!==this.run||typeof value.node_id!=='string'||typeof value.type!=='string'||!value.type||typeof value.at!=='string'||!value.data||typeof value.data!=='object'||Array.isArray(value.data))throw new StreamFailure("development.events.run_event_mismatch_or_incomplete_content",400);
  this.accept(value as RunEvent);
 }
}

export class RunEventWindow{
 cursor=0;items:RunEvent[]=[];limited=false;
 private bytes=0;private sizes:number[]=[];
 constructor(private run:string,private limit=300){}
 add(event:RunEvent){if(event.run_id!==this.run||!Number.isSafeInteger(event.sequence)||event.sequence<=this.cursor)return false;const size=encoder.encode(JSON.stringify(event)).byteLength;if(size>maxEventBytes)throw new StreamFailure("development.events.a_single_event_exceeds_the_16_mib",400);this.cursor=event.sequence;this.items.push(event);this.sizes.push(size);this.bytes+=size;
  while(this.items.length>this.limit||this.bytes>maxEventBytes){this.items.shift();this.bytes-=this.sizes.shift()!;this.limited=true;}return true;}
}

export function currentRunSnapshot<T extends {id:string;version:number}>(current:T|null,incoming:T,run:string){return incoming.id===run&&(!current||current.id!==run||incoming.version>=current.version);}

export function reconnectDelay(attempt:number,random= Math.random){return Math.min(10000,Math.round(Math.min(8000,500*2**Math.min(5,Math.max(0,attempt)))*(0.5+Math.max(0,Math.min(1,random())))));}
function abortWait(ms:number,signal:AbortSignal){return new Promise<void>(resolve=>{if(signal.aborted){resolve();return;}const done=()=>{clearTimeout(timer);signal.removeEventListener('abort',done);resolve();};const timer=setTimeout(done,ms);signal.addEventListener('abort',done,{once:true});});}

export async function connectRunEvents(options:{project:string;run:string;signal:AbortSignal;onEvent:(event:RunEvent)=>void;onState:(state:RunConnectionState)=>void;fetcher?:typeof fetch;wait?:(ms:number,signal:AbortSignal)=>Promise<void>;random?:()=>number}){
 const {project,run,signal,onEvent,onState}=options;let cursor=0,attempt=0,reconnectMessage='';let reconnectDiagnostic:UiError|undefined;
 const state=(next:RunConnectionState)=>{if(!signal.aborted)onState(next);};
 while(!signal.aborted){state({state:attempt||reconnectMessage?'reconnecting':'connecting',attempt,message:reconnectMessage,diagnostic:reconnectDiagnostic});const connection=new AbortController();let reader:ReadableStreamDefaultReader<Uint8Array>|undefined;
  const abort=()=>{connection.abort();void reader?.cancel().catch(()=>{});};signal.addEventListener('abort',abort,{once:true});if(signal.aborted)abort();
  // The server rotates at 20s. This also bounds a silent proxy/half-open read.
  const timeout=setTimeout(abort,30000);
  try{
   const response=await(options.fetcher||fetch)('/api/v1/runs/'+encodeURIComponent(run)+'/events/stream?project_id='+encodeURIComponent(project)+'&after='+cursor,{credentials:'same-origin',headers:{Accept:'text/event-stream'},signal:connection.signal});
   if(signal.aborted)return;if(!response.ok)throw new StreamFailure("development.events.request_failed",response.status,{status:response.status});
   if(!response.headers.get('Content-Type')?.startsWith('text/event-stream')||!response.body)throw new StreamFailure("development.events.invalid_live_event_response_format",400);
   reader=response.body.getReader();state({state:'live'});let accepted=false;const connectedAt=Date.now();
   const parser=new RunEventParser(run,event=>{if(signal.aborted||event.sequence<=cursor)return;onEvent(event);cursor=event.sequence;accepted=true;});
   while(!signal.aborted){const chunk=await reader.read();if(chunk.done){parser.end();break;}parser.push(chunk.value);}
   if(signal.aborted)return;attempt=accepted||Date.now()-connectedAt>=10000?0:attempt+1;reconnectDiagnostic=new UiError("development.events.resuming_connection");reconnectMessage=reconnectDiagnostic.message;
  }catch(error){if(signal.aborted)return;const failure=error instanceof StreamFailure?error:new StreamFailure("development.events.live_connection_interrupted");
   if(failure.status===401||failure.status===403){state({state:'forbidden',message:failure.status===401?t("development.events.your_session_expired_sign_in_again"):t("development.events.access_to_this_run_has_expired"),diagnostic:new UiError(failure.status===401?"development.events.your_session_expired_sign_in_again":"development.events.access_to_this_run_has_expired")});return;}
   if(failure.status===400||failure.status===404||failure.status===410||failure.status===422){state({state:'invalid',message:failure.message+t("development.events.live_events_stopped_check_persistent_history_then"),diagnostic:failure,stopped:true});return;}
   attempt++;reconnectDiagnostic=failure;reconnectMessage=failure.message;state({state:'reconnecting',attempt,message:failure.message,diagnostic:failure});
  }finally{clearTimeout(timeout);signal.removeEventListener('abort',abort);connection.abort();try{await reader?.cancel();}catch{/* The fetch body may already be closed. */}reader?.releaseLock();}
  if(!signal.aborted){state({state:'reconnecting',attempt,message:reconnectMessage,diagnostic:reconnectDiagnostic});await(options.wait||abortWait)(reconnectDelay(attempt,options.random),signal);}
 }
}

// Notifications arriving during a snapshot read require one trailing read.
// A single in-flight read also prevents an older response replacing newer state.
export class SnapshotRefreshQueue{
 private pending=false;private active=false;private closed=false;private timer:ReturnType<typeof setTimeout>|undefined;
 constructor(private refresh:()=>Promise<void>,private delay=120){}
 request(){if(this.closed)return;this.pending=true;if(!this.active&&!this.timer)this.timer=setTimeout(()=>void this.drain(),this.delay);}
 close(){this.closed=true;this.pending=false;clearTimeout(this.timer);this.timer=undefined;}
 private async drain(){this.timer=undefined;if(this.closed||this.active)return;this.active=true;try{while(this.pending&&!this.closed){this.pending=false;await this.refresh();}}finally{this.active=false;}}
}
