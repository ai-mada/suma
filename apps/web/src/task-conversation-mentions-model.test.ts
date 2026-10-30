import {describe, expect, it} from 'vitest';
import {conversationDraftKey, readConversationDraft, saveConversationDraft, validMentionResourceIDs,
  validMentionTargetPage} from './task-conversation-mentions-model';
import {validConversationMessage, validConversationReceipt, validConversationRequest} from './task-conversation-model';
import type {WriteRecoveryRequest} from './write-action';

const message = {id:'m',sequence:1,task_version:2,body:'Plain @Ada text',author_id:'u',author_name:'Author',actor_kind:'human',
  created_at:'2026-10-29T00:00:00Z',requirement_id:'',requirement_version:0,run_id:'',reply_id:'',reply_to_sequence:0,execution_run_id:''};
const mention = {resource_id:'human-a',user_id:'person-a',name_at_post:'Recorded name'};
const request = (ids?:string[]):WriteRecoveryRequest => ({path:'/task-conversations',method:'POST',idempotency_key:'original-key',
  body:{project_id:'p',task_id:'t',expected_task_version:2,body:message.body,idempotency_key:'original-key',...(ids?{mention_resource_ids:ids}:{})}});

describe('explicit task conversation mention identity',()=>{
  it('preserves plain originals and verifies exact selected recipients in the receipt',()=>{
    expect(validConversationMessage(message)).toBe(true);
    expect(validConversationRequest(request(),'p','t')).toBe(true);
    expect(validConversationReceipt(message,request(),'u')).toBe(true);
    expect(validConversationMessage({...message,mentions:[mention]})).toBe(true);
    expect(validConversationReceipt({...message,mentions:[mention]},request(['human-a']),'u')).toBe(true);
    expect(validConversationReceipt({...message,mentions:[mention]},request(['human-b']),'u')).toBe(false);
    expect(validConversationReceipt(message,request(['human-a']),'u')).toBe(false);
    expect(validConversationMessage({...message,mentions:[mention,mention]})).toBe(false);
    expect(validConversationMessage({...message,actor_kind:'assistant',mentions:[mention]})).toBe(false);
    expect(validMentionResourceIDs(['a','a'])).toBe(false);
    expect(validMentionResourceIDs(Array.from({length:21},(_,n)=>'human-'+n))).toBe(false);
  });
  it('keeps only authorized selected targets without silently replacing unavailable IDs',()=>{
    const page={project_id:'p',task_id:'t',limit:20,items:[{resource_id:'a',user_id:'u',name:'Ada',resource_version:3}],total:1,next_cursor:''};
    expect(validMentionTargetPage(page,'p','t',['a','missing'])).toBe(true);
    expect(validMentionTargetPage({...page,project_id:'other'},'p','t')).toBe(false);
    expect(validMentionTargetPage({...page,items:[page.items[0],page.items[0]],total:2},'p','t')).toBe(false);
    expect(validMentionTargetPage({...page,next_cursor:'more'},'p','t',['a'])).toBe(false);
    expect(validMentionTargetPage({...page,items:[{...page.items[0],resource_id:'not-selected'}]},'p','t',['a'])).toBe(false);
  });
  it('isolates durable body and IDs by owner and task and stores no directory names',()=>{
    const values=new Map<string,string>(),storage={getItem:(key:string)=>values.get(key)??null,setItem:(key:string,value:string)=>{values.set(key,value);},removeItem:(key:string)=>{values.delete(key);}};
    const key=conversationDraftKey('u','o','p','t');
    saveConversationDraft(storage,key,{body:'  draft 🧪  ',mention_resource_ids:['a']});
    expect(readConversationDraft(storage,key).draft).toEqual({body:'  draft 🧪  ',mention_resource_ids:['a']});
    expect(readConversationDraft(storage,conversationDraftKey('other','o','p','t')).draft.body).toBe('');
    expect(readConversationDraft(storage,conversationDraftKey('u','o','p','other')).draft.mention_resource_ids).toEqual([]);
    expect(values.get(key)).not.toContain('name');
    values.set(key,'{"format":"unknown","body":"private original"}');
    expect(readConversationDraft(storage,key).invalid).toBe(true);
    expect(values.get(key)).toContain('private original');
  });
});
