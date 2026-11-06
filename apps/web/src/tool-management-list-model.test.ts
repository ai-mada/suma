import {expect,it} from 'vitest';
import type {ApiCaller} from './api';
import {ApiError} from './api';
import {ToolProfilePager,readToolListFilters,retainToolListSearchParams,toolListSearchParams,validToolListQuery} from './tool-management-list-model';

it('keeps tool filters separate from task navigation and rejects oversized or invalid public queries',()=>{
 const params=new URLSearchParams('project=p&status=running&week=2026-11-01&tools_state=revoked&tools_query=Codex');
 expect(readToolListFilters(params)).toEqual({state:'revoked',query:'Codex'});
 const next=toolListSearchParams(params,{state:'active',query:' team '});
 expect(next.get('status')).toBe('running');expect(next.get('week')).toBe('2026-11-01');expect(next.has('tools_state')).toBe(false);expect(next.get('tools_query')).toBe('team');
 expect(readToolListFilters(new URLSearchParams('tools_state=bogus&tools_query=x%00'))).toEqual({state:'active',query:''});
 expect(validToolListQuery('中'.repeat(40))).toBe(true);expect(validToolListQuery('中'.repeat(41))).toBe(false);
});

it('binds pages to the original filters and caller, drops late responses, and clears protected rows on current refusal',async()=>{
 const calls:string[]=[],pending:Array<(value:unknown)=>void>=[];
 let refused=false;
 const caller=(async(path:string):Promise<unknown>=>{calls.push(path);if(refused)throw new ApiError(403,'forbidden','Forbidden');return new Promise<unknown>(resolve=>pending.push(resolve));}) as ApiCaller;
 const row=(id:string,worker_id='worker-a',project_id='p')=>({id,worker_id,project_id});
 const pager=new ToolProfilePager<ReturnType<typeof row>>('p',{state:'active',query:''},caller);
 const first=pager.first();pending.shift()!({items:[row('old')],total:2,next_cursor:'active-cursor'});await first;
 const more=pager.more();expect(calls.at(-1)).toContain('cursor=active-cursor');
 pager.setScope('p',{state:'revoked',query:'Codex'});expect(pager.snapshot().items).toBeUndefined();
 const fresh=pager.first();expect(calls.at(-1)).toContain('state=revoked&query=Codex');expect(calls.at(-1)).not.toContain('cursor=');
 pending.shift()!({items:[row('late-old')],total:2});await more;expect(pager.snapshot().items).toBeUndefined();
 pending.shift()!({items:[row('current')],total:1});await fresh;expect(pager.snapshot().items).toEqual([row('current')]);
 refused=true;await pager.first();expect(pager.snapshot().items).toBeUndefined();expect(pager.snapshot().next_cursor).toBe('');
});

it('keeps the exact worker in deep links and rejects cross-worker or cross-project pages after changing nodes',async()=>{
 const params=new URLSearchParams('project=p&view=settings&sub=tools&tools_worker=Codex.Node&tools_state=all');
 const filters={state:'all',query:'',worker:'Codex.Node'} as const;
 expect(readToolListFilters(params)).toEqual(filters);
 expect(readToolListFilters(new URLSearchParams('tools_worker=%20Codex.Node')).worker).toBeUndefined();
 expect(readToolListFilters(new URLSearchParams('tools_worker=first&tools_worker=second')).worker).toBeUndefined();
 expect(toolListSearchParams(new URLSearchParams(),filters).get('tools_worker')).toBe('Codex.Node');
 expect(toolListSearchParams(params,{state:'all',query:''}).has('tools_worker')).toBe(false);
 const detail=retainToolListSearchParams(params,new URLSearchParams('project=p&view=settings&sub=tools&record=detail'));
 expect(detail.get('tools_worker')).toBe('Codex.Node');
 expect(detail.get('record')).toBe('detail');
 expect(retainToolListSearchParams(params,new URLSearchParams('project=other&view=settings&sub=tools')).has('tools_worker')).toBe(false);
 const calls:URLSearchParams[]=[],pending:Array<(value:unknown)=>void>=[];
 const caller=(async(path:string):Promise<unknown>=>{calls.push(new URLSearchParams(path.split('?')[1]));return new Promise(resolve=>pending.push(resolve));}) as ApiCaller;
 const row=(id:string,worker_id:string,project_id='p')=>({id,worker_id,project_id});
 const pager=new ToolProfilePager<ReturnType<typeof row>>('p',filters,caller);
 const first=pager.first();
 expect(calls.at(-1)?.get('worker_id')).toBe('Codex.Node');
 pending.shift()!({items:[row('first','Codex.Node')],total:2,next_cursor:'exact-cursor'});expect(await first).toBe(true);
 const more=pager.more();
 expect(calls.at(-1)?.get('worker_id')).toBe('Codex.Node');
 expect(calls.at(-1)?.get('cursor')).toBe('exact-cursor');
 pager.setScope('p',{...filters,worker:'codex.node'});
 const fresh=pager.first();
 expect(calls.at(-1)?.get('worker_id')).toBe('codex.node');
 expect(calls.at(-1)?.has('cursor')).toBe(false);
 pending.shift()!({items:[row('late','Codex.Node')],total:2});expect(await more).toBe(false);
 pending.shift()!({items:[row('other-case','Codex.Node')],total:1});expect(await fresh).toBe(false);
 expect(pager.snapshot().items).toBeUndefined();
 const wrongProject=pager.first();
 pending.shift()!({items:[row('other-project','codex.node','another')],total:1});expect(await wrongProject).toBe(false);
 expect(pager.snapshot().items).toBeUndefined();
});
