// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
import {TurnSteeringCard,TurnSteeringFacts,type TurnSteeringCardProps} from './TurnSteeringCard';
import {turnSteeringContext,turnSteeringInstruction,turnSteeringMessage,turnSteeringSession} from './turn-steering.test-support';
import {setLocale} from './i18n';

let root:Root|undefined,host:HTMLDivElement|undefined;
afterEach(async()=>{if(root)await act(async()=>root!.unmount());host?.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function setup(){await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);}
async function click(selector:string){const target=host!.querySelector<HTMLElement>(selector);expect(target,selector).not.toBeNull();await act(async()=>target!.click());}

it('requires the original human message and an explicit exact target, never resends an existing instruction and preserves original pending recovery',async()=>{
 await setup();const fetch=vi.fn(),onSelect=vi.fn(),onSend=vi.fn(),onRestore=vi.fn(),onRefresh=vi.fn();vi.stubGlobal('fetch',fetch);
 const render=async(patch:Partial<TurnSteeringCardProps>={})=>{await act(async()=>root!.render(<TurnSteeringCard context={turnSteeringContext} selectedSessionID="" onSelect={onSelect} onSend={onSend} onRestore={onRestore} onRefresh={onRefresh} {...patch}/>));};
 await render();expect(host!.querySelector('[data-turn-steering-message]')?.textContent).toBe(turnSteeringMessage.body);expect(host!.querySelector<HTMLInputElement>('[data-turn-steering-target]')?.checked).toBe(false);expect(host!.querySelector<HTMLButtonElement>('[data-turn-steering-send]')?.disabled).toBe(true);expect(onSelect).not.toHaveBeenCalled();expect(onSend).not.toHaveBeenCalled();
 await click('[data-turn-steering-target="'+turnSteeringSession.id+'"]');expect(onSelect).toHaveBeenCalledExactlyOnceWith(turnSteeringSession.id);await render({selectedSessionID:turnSteeringSession.id});expect(host!.querySelector<HTMLButtonElement>('[data-turn-steering-send]')?.disabled).toBe(false);await click('[data-turn-steering-send]');expect(onSend).toHaveBeenCalledExactlyOnceWith(turnSteeringSession.id);
 await render({selectedSessionID:turnSteeringSession.id,stale:true});expect(host!.querySelector<HTMLButtonElement>('[data-turn-steering-send]')?.disabled).toBe(true);
 await render({selectedSessionID:turnSteeringSession.id,context:{...turnSteeringContext,instructions:[turnSteeringInstruction]}});expect(host!.querySelector<HTMLButtonElement>('[data-turn-steering-send]')?.disabled).toBe(true);
 await render({context:{...turnSteeringContext,can_send:false,reason:'not_own_message'}});expect(host!.textContent).toContain('Only your own saved human message');expect(host!.querySelector<HTMLButtonElement>('[data-turn-steering-send]')?.disabled).toBe(true);
 await render({disabled:true,readOnly:true,pending:true,context:{...turnSteeringContext,targets:[{...turnSteeringSession,state:'closed',allowed_actions:[]}],can_send:false,reason:'no_live_turn'}});expect(host!.querySelector('[data-turn-steering-send]')).toBeNull();expect(host!.querySelector<HTMLButtonElement>('[data-turn-steering-restore]')?.disabled).toBe(false);await click('[data-turn-steering-restore]');expect(onRestore).toHaveBeenCalledTimes(1);expect(onSend).toHaveBeenCalledTimes(1);expect(host!.querySelector<HTMLButtonElement>('[data-turn-steering-refresh]')?.disabled).toBe(false);await click('[data-turn-steering-refresh]');expect(onRefresh).toHaveBeenCalledTimes(1);await render({pending:true,stale:true});expect(host!.querySelector<HTMLButtonElement>('[data-turn-steering-restore]')?.disabled).toBe(true);expect(host!.querySelector<HTMLButtonElement>('[data-turn-steering-refresh]')?.disabled).toBe(false);
 await render({disabled:true,error:new Error('Storage unavailable')});expect(host!.querySelector<HTMLButtonElement>('[data-turn-steering-send]')?.disabled).toBe(true);expect(host!.querySelector<HTMLButtonElement>('[data-turn-steering-refresh]')?.disabled).toBe(false);await click('[data-turn-steering-refresh]');expect(onRefresh).toHaveBeenCalledTimes(2);expect(fetch).not.toHaveBeenCalled();
});

it('keeps platform storage, write facts, native acknowledgement and closed-before-write separate without offering delivery retries',async()=>{
 await setup();const render=async(instruction=turnSteeringInstruction,session=turnSteeringSession)=>{await act(async()=>root!.render(<TurnSteeringFacts instruction={instruction} session={session}/>));};
 await render();expect(host!.querySelector('[data-turn-steering-status="saved"]')).not.toBeNull();expect(host!.querySelector('[data-turn-steering-status="observed"]')).toBeNull();expect(host!.querySelector<HTMLDetailsElement>('details')?.open).toBe(false);
 const observation={sequence:1,request_id:{kind:'number',value:'9007199254740993'},params_sha256:'5'.repeat(64),observed:true,write_started:true,write_written:false,native_accepted:false,native_rejected:false,turn_closed:false};
 await render({...turnSteeringInstruction,observation});expect(host!.querySelector('[data-turn-steering-status="observed"]')).not.toBeNull();expect(host!.querySelector('[data-turn-steering-status="started"]')).not.toBeNull();expect(host!.querySelector('[data-turn-steering-status="written"]')).toBeNull();
 await render({...turnSteeringInstruction,observation:{...observation,sequence:2,write_written:true,native_accepted:true}});expect(host!.querySelector('[data-turn-steering-status="written"]')).not.toBeNull();expect(host!.querySelector('[data-turn-steering-status="accepted"]')).not.toBeNull();expect(host!.textContent).toContain('does not prove model compliance');
 await render({...turnSteeringInstruction,observation:{...observation,sequence:3,native_rejected:true}});expect(host!.querySelector('[data-turn-steering-status="rejected"]')).not.toBeNull();expect(host!.querySelector('[data-turn-steering-send]')).toBeNull();
 await render({...turnSteeringInstruction,observation:{...observation,sequence:4,failure_code:'steering_write_unknown'}});expect(host!.querySelector('[data-turn-steering-status="unknown"]')).not.toBeNull();expect(host!.querySelector('[data-turn-steering-send]')).toBeNull();
 const closed={...turnSteeringSession,state:'closed',allowed_actions:[]};await render(turnSteeringInstruction,closed);expect(host!.querySelector('[data-turn-steering-status="not_written"]')).not.toBeNull();expect(host!.querySelector('[data-turn-steering-status="accepted"]')).toBeNull();expect(host!.querySelector('[data-turn-steering-status="unknown"]')).toBeNull();await render(turnSteeringInstruction,{...closed,fence:closed.fence+1});expect(host!.querySelector('[data-turn-steering-status="not_written"]')).toBeNull();
});
