import {expect,it} from 'vitest';
import type {ApiCaller} from './api';
import {ApiError} from './api';
import {ToolProfilePager,readToolListFilters,toolListSearchParams,validToolListQuery} from './tool-management-list-model';

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
 const pager=new ToolProfilePager<{id:string}>('p',{state:'active',query:''},caller);
 const first=pager.first();pending.shift()!({items:[{id:'old'}],total:2,next_cursor:'active-cursor'});await first;
 const more=pager.more();expect(calls.at(-1)).toContain('cursor=active-cursor');
 pager.setScope('p',{state:'revoked',query:'Codex'});expect(pager.snapshot().items).toBeUndefined();
 const fresh=pager.first();expect(calls.at(-1)).toContain('state=revoked&query=Codex');expect(calls.at(-1)).not.toContain('cursor=');
 pending.shift()!({items:[{id:'late-old'}],total:2});await more;expect(pager.snapshot().items).toBeUndefined();
 pending.shift()!({items:[{id:'current'}],total:1});await fresh;expect(pager.snapshot().items).toEqual([{id:'current'}]);
 refused=true;await pager.first();expect(pager.snapshot().items).toBeUndefined();expect(pager.snapshot().next_cursor).toBe('');
});
