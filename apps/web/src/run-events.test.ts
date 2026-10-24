import {describe,it,expect,vi} from 'vitest';
import {RunEventParser,RunEventWindow,reconnectDelay,connectRunEvents,SnapshotRefreshQueue,currentRunSnapshot} from './run-events';

const event=(sequence:number,run_id='r')=>({sequence,run_id,node_id:'n',type:'node_progress',at:'2026-10-24T00:00:00Z',data:{output:'中文输出'}});
const frame=(sequence:number,run='r')=>`id: ${sequence}\nevent: run_event\ndata: ${JSON.stringify(event(sequence,run))}\n\n`;
const body=(text:string)=>new ReadableStream<Uint8Array>({start(c){c.enqueue(new TextEncoder().encode(text));c.close();}});

describe('durable run event frames',()=>{
 it('waits for complete frames across UTF-8, CRLF and multi-line data chunks',()=>{
  const received:any[]=[];const parser=new RunEventParser('r',e=>received.push(e));
  const bytes=new TextEncoder().encode('\ufeff: heartbeat\r\nid: 12\r\nevent: run_event\r\ndata: {"sequence":12,"run_id":"r","node_id":"n",\r\ndata: "type":"node_progress","at":"2026-10-24T00:00:00Z","data":{"output":"中文"}}\r\n\r\n');
  for(const byte of bytes.slice(0,-1))parser.push(Uint8Array.of(byte));
  expect(received).toHaveLength(1);expect(received[0].data.output).toBe('中文');
  parser.push(bytes.slice(-1));expect(received).toHaveLength(1);
 });
 it('discards a final unterminated frame and never advances a cursor for it',()=>{
  const received:any[]=[];const parser=new RunEventParser('r',e=>received.push(e));parser.push(new TextEncoder().encode(frame(1)+frame(2).trimEnd()));parser.end();expect(received.map(x=>x.sequence)).toEqual([1]);
 });
 it.each([frame(1,'other'), 'id: 1\nevent: run_event\ndata: {"sequence":2}\n\n','id: 9007199254740992\nevent: run_event\ndata: {}\n\n'])('rejects malformed/cross-run frames without delivering them',raw=>{
  const received:any[]=[];const parser=new RunEventParser('r',e=>received.push(e));expect(()=>parser.push(new TextEncoder().encode(raw))).toThrow();expect(received).toEqual([]);
 });
 it('ignores comments and unrelated event names',()=>{const received:any[]=[];const p=new RunEventParser('r',e=>received.push(e));p.push(new TextEncoder().encode(':ping\n\nevent: unrelated\ndata: {}\n\n'+frame(8)));expect(received.map(e=>e.sequence)).toEqual([8]);});
 it('deduplicates and orders global sequence gaps and bounds the visible window',()=>{const w=new RunEventWindow('r',2);expect(w.add(event(2))).toBe(true);expect(w.add(event(8))).toBe(true);expect(w.add(event(2))).toBe(false);expect(w.add(event(5))).toBe(false);expect(w.add(event(9,'other'))).toBe(false);w.add(event(11));expect(w.cursor).toBe(11);expect(w.items.map(e=>e.sequence)).toEqual([8,11]);expect(w.limited).toBe(true);});
 it('accepts a near-2-MiB command after Go JSON HTML escaping expands its persisted event sixfold',()=>{
  // http.go accepts 2 MiB JSON requests; encoding/json escapes every < as \\u003c.
  const reason='<'.repeat(2*1024*1024-512),received:any[]=[];
  const json=JSON.stringify({...event(7),type:'command_pause',data:{reason}}).replaceAll('<','\\u003c');
  const bytes=new TextEncoder().encode('id: 7\nevent: run_event\ndata: '+json+'\n\n');
  const parser=new RunEventParser('r',value=>received.push(value));
  for(let at=0;at<bytes.length;at+=65537)parser.push(bytes.subarray(at,at+65537));
  parser.end();expect(received).toHaveLength(1);expect(received[0].data.reason).toBe(reason);
 });
 it('bounds retained event bytes as well as count while preserving the latest accepted event',()=>{
  const window=new RunEventWindow('r');
  for(let n=1;n<=20;n++)window.add({...event(n),data:{reason:'x'.repeat(1100000)}});
  expect(window.items.length).toBeLessThanOrEqual(15);expect(window.items.at(-1)?.sequence).toBe(20);expect(window.cursor).toBe(20);expect(window.limited).toBe(true);
 });
 it('includes line framing in the byte budget across a multi-line frame',()=>{
  const parser=new RunEventParser('r',()=>{throw Error('unexpected event')}),comment=new TextEncoder().encode(':'+'x'.repeat(1023)+'\n');
  expect(()=>{for(let n=0;n<16384;n++)parser.push(comment);}).toThrow('16 MiB');
 });
 it('caps exponential reconnect jitter',()=>{expect(reconnectDelay(0,()=>0)).toBeGreaterThanOrEqual(250);expect(reconnectDelay(99,()=>1)).toBeLessThanOrEqual(10000);});
 it('keeps newer command snapshots and rejects another selected run',()=>{expect(currentRunSnapshot({id:'r',version:9},{id:'r',version:8},'r')).toBe(false);expect(currentRunSnapshot(null,{id:'old',version:10},'r')).toBe(false);expect(currentRunSnapshot({id:'r',version:9},{id:'r',version:10},'r')).toBe(true);});
});

describe('run event connection lifecycle',()=>{
 it('reconnects from only the last complete accepted frame and stops on HTTP 403',async()=>{
  const states:string[]=[];const urls:string[]=[];const received:number[]=[];let request=0;
  await connectRunEvents({project:'p',run:'r',signal:new AbortController().signal,onEvent:e=>received.push(e.sequence),onState:s=>states.push(s.state),wait:async()=>{},fetcher:async(input,init)=>{
   urls.push(String(input));expect(init?.credentials).toBe('same-origin');expect(String(input)).not.toContain('token');request++;
   return request===1?new Response(body(frame(3)+frame(4).trimEnd()),{headers:{'Content-Type':'text/event-stream'}}):new Response('{}',{status:403});
  }});
  expect(received).toEqual([3]);expect(urls[1]).toContain('after=3');expect(states.at(-1)).toBe('forbidden');
 });
 it('aborts an open read without late state or event callbacks',async()=>{
  const abort=new AbortController();const states:string[]=[];let cancelled=false;
  const connection=connectRunEvents({project:'p',run:'r',signal:abort.signal,onEvent:()=>{throw Error('late event')},onState:s=>states.push(s.state),fetcher:async()=>new Response(new ReadableStream({cancel(){cancelled=true}}),{headers:{'Content-Type':'text/event-stream'}})});
  await vi.waitFor(()=>expect(states).toContain('live'));abort.abort();await connection;expect(cancelled).toBe(true);expect(states.at(-1)).toBe('live');
 });
 it('reports a stream revocation frame and ends without reconnecting',async()=>{const states:string[]=[];await connectRunEvents({project:'p',run:'r',signal:new AbortController().signal,onEvent:()=>{},onState:s=>states.push(s.state),fetcher:async()=>new Response(body('event: stream_error\ndata: {"status":403,"code":"forbidden"}\n\n'),{headers:{'Content-Type':'text/event-stream'}})});expect(states.at(-1)).toBe('forbidden');});
 it('accepts normalized empty metadata on a final server event and resumes after its durable cursor',async()=>{
  const urls:string[]=[],received:any[]=[];
  const terminal={...event(12),node_id:'',type:'run_succeeded',data:{}};
  await connectRunEvents({project:'p',run:'r',signal:new AbortController().signal,onEvent:value=>received.push(value),onState:()=>{},wait:async()=>{},fetcher:async input=>{
   urls.push(String(input));return urls.length===1?new Response(body(frame(11)+'id: 12\nevent: run_event\ndata: '+JSON.stringify(terminal)+'\n\n'),{headers:{'Content-Type':'text/event-stream'}}):new Response('{}',{status:403});
  }});
  expect(received.map(value=>value.sequence)).toEqual([11,12]);expect(received[1]).toEqual(terminal);expect(urls[1]).toContain('after=12');
 });
 it('rejects null metadata as a wire contract violation without advancing past it',async()=>{
  const states:string[]=[],received:number[]=[];let requests=0;
  const invalid={...event(12),node_id:'',type:'run_succeeded',data:null};
  await connectRunEvents({project:'p',run:'r',signal:new AbortController().signal,onEvent:value=>received.push(value.sequence),onState:value=>states.push(value.state),fetcher:async()=>{
   requests++;return new Response(body(frame(11)+'id: 12\nevent: run_event\ndata: '+JSON.stringify(invalid)+'\n\n'+frame(13)),{headers:{'Content-Type':'text/event-stream'}});
  }});
  expect(received).toEqual([11]);expect(requests).toBe(1);expect(states.at(-1)).toBe('invalid');
 });
 it.each([400,'malformed'] as const)('stops a persistent invalid cursor/frame (%s) without blind reconnects',async failure=>{
  const states:string[]=[],received:number[]=[],abort=new AbortController();let requests=0;
  await connectRunEvents({project:'p',run:'r',signal:abort.signal,onEvent:value=>received.push(value.sequence),onState:value=>states.push(value.state),
   fetcher:async()=>{requests++;return failure===400?new Response('{}',{status:400}):new Response(body('id: 4\nevent: run_event\ndata: {broken}\n\n'),{headers:{'Content-Type':'text/event-stream'}});},
   wait:async()=>{if(requests===2)abort.abort();}});
  expect(received).toEqual([]);expect(requests).toBe(1);expect(states.at(-1)).toBe('invalid');
 });
 it('stops on a UTF-8 event larger than the shared byte budget before advancing its cursor',async()=>{
  const states:string[]=[],received:number[]=[],abort=new AbortController();let requests=0;
  // Less than 16 Mi UTF-16 code units, but more than 16 MiB of UTF-8.
  const oversized='id: 9\nevent: run_event\ndata: '+JSON.stringify({...event(9),data:{reason:'中'.repeat(6*1024*1024)}})+'\n\n';
  await connectRunEvents({project:'p',run:'r',signal:abort.signal,onEvent:value=>received.push(value.sequence),onState:value=>states.push(value.state),fetcher:async()=>{requests++;return new Response(body(oversized),{headers:{'Content-Type':'text/event-stream'}});},wait:async()=>{abort.abort();}});
  expect(received).toEqual([]);expect(requests).toBe(1);expect(states.at(-1)).toBe('invalid');
 });
 it('merges burst refreshes and performs a trailing read for notifications during an in-flight snapshot',async()=>{
  let release!:()=>void;let reads=0;const queue=new SnapshotRefreshQueue(async()=>{reads++;if(reads===1)await new Promise<void>(r=>release=r)},0);
  queue.request();queue.request();await vi.waitFor(()=>expect(reads).toBe(1));queue.request();release();await vi.waitFor(()=>expect(reads).toBe(2));queue.close();queue.request();await new Promise(r=>setTimeout(r,5));expect(reads).toBe(2);
 });
});
