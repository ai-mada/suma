import {expect, it} from 'vitest';
import {conversationKnowledgeLimit, conversationKnowledgeUnion, sameKnowledgePin, validConversationKnowledge, validKnowledgePins,
  validKnowledgeRevisionDetail, validKnowledgeContext} from './task-conversation-knowledge-model';
import {validReplySource} from './task-conversation-reply-model';
import {validConversationReceipt, validConversationRequest} from './task-conversation-model';
import {conversationDraftKey, readConversationDraft, saveConversationDraft} from './task-conversation-mentions-model';
import type {WriteRecoveryRequest} from './write-action';

const pin = {id:'knowledge-a',revision:2,sha256:'a'.repeat(64)};
const metadata = {...pin,title_at_post:'Original approved title',kind_at_post:'spec'};
const message = {id:'m',sequence:1,task_version:2,body:'Original note',author_id:'u',author_name:'Author',actor_kind:'human',
  created_at:'2026-10-29T00:00:00Z',requirement_id:'',requirement_version:0,run_id:'',reply_id:'',reply_to_sequence:0,execution_run_id:'',knowledge:[metadata]};
const request:WriteRecoveryRequest={path:'/task-conversations',method:'POST',idempotency_key:'original-operation',body:{project_id:'p',task_id:'t',expected_task_version:2,body:message.body,knowledge:[pin],idempotency_key:'original-operation'}};
const snapshot={revision:2,based_on_revision:1,state:'published',kind:'spec',title:'Original approved title',summary:'',content:'Exact historical body',sha256:pin.sha256,sources:[],method:'manual',created_by:'u',created_at:'2026-10-29T00:00:00Z'};

it('binds the original request and receipt to exact unique pins without caller snapshots',()=>{
  expect(validConversationRequest(request,'p','t')).toBe(true);
  expect(validConversationReceipt(message,request,'u')).toBe(true);
  expect(validConversationReceipt({...message,knowledge:[{...metadata,revision:3}]},request,'u')).toBe(false);
  expect(validConversationReceipt(message,{...request,body:{...request.body,knowledge:[{sha256:pin.sha256,revision:pin.revision,id:pin.id}]}},'u')).toBe(true);
  expect(validKnowledgePins([pin,pin])).toBe(false);
  expect(validKnowledgePins([{...pin,title:'caller snapshot'}])).toBe(false);
  expect(validKnowledgePins(Array.from({length:conversationKnowledgeLimit+1},(_,i)=>({...pin,id:'k'+i})))).toBe(false);
  expect(validConversationKnowledge([{...metadata,sha256:pin.sha256.toUpperCase()}])).toBe(false);
  expect(validConversationKnowledge([{...metadata,kind_at_post:['spec']}])).toBe(false);
  expect(sameKnowledgePin(pin,{...pin,revision:3})).toBe(false);
});
it('reads only the requested historical tuple, regardless of present publication eligibility',()=>{
  const detail={project_id:'p',knowledge_id:pin.id,snapshot};
  expect(validKnowledgeRevisionDetail(detail,'p',pin)).toBe(true);
  for(const changed of [{...detail,project_id:'other'},{...detail,knowledge_id:'other'},
    {...detail,snapshot:{...snapshot,revision:3}},{...detail,snapshot:{...snapshot,sha256:'b'.repeat(64)}}])
    expect(validKnowledgeRevisionDetail(changed,'p',pin)).toBe(false);
  expect(validKnowledgeContext([{...pin,kind:'spec',title:snapshot.title,summary:'',content:snapshot.content,sources:[]}])).toBe(true);
  expect(validKnowledgeContext([{...pin,kind:'spec',title:snapshot.title,summary:'',content:snapshot.content,sources:null}])).toBe(false);
});
it('persists only body, recipient IDs and exact pins under the original draft owner',()=>{
  const data=new Map<string,string>(),storage={getItem:(key:string)=>data.get(key)??null,setItem:(key:string,value:string)=>{data.set(key,value);},removeItem:(key:string)=>{data.delete(key);}};
  const key=conversationDraftKey('u','o','p','t');
  saveConversationDraft(storage,key,{body:' original ',mention_resource_ids:['human-a'],knowledge:[pin]});
  expect(readConversationDraft(storage,key).draft).toEqual({body:' original ',mention_resource_ids:['human-a'],knowledge:[pin]});
  expect(data.get(key)).not.toContain('title');
  expect(readConversationDraft(storage,conversationDraftKey('other','o','p','t')).draft.body).toBe('');
  data.set(key,JSON.stringify({format:'suma-task-conversation-draft-v1',body:'keep',mention_resource_ids:[],knowledge:[pin,pin]}));
  expect(readConversationDraft(storage,key).invalid).toBe(true);expect(data.get(key)).toContain('keep');
});
it('checks the frozen message union including assistant-only carried references and rejects conflicting revisions',()=>{
  const assistant={...message,id:'assistant',sequence:2,actor_kind:'assistant',reply_id:'original-reply',reply_to_sequence:1,execution_run_id:'original-run'};
  const context={...pin,kind:'spec',title:snapshot.title,summary:'',content:snapshot.content,sources:[]};
  const {knowledge:_knowledge,...plain}=message;
  const followup={...plain,id:'followup',sequence:3};
  const source={sha256:'d'.repeat(64),task:{id:'t',version:2,title:'Task',description:'',acceptance_criteria:[]},requirement:null,task_run_id:'',
    messages:[assistant,followup],coverage:{total:3,included:2,first_sequence:2,last_sequence:3,complete:false},reply_to_sequence:3,
    execution_route:{tenant_id:'o',project_id:'p',version:0,pool_id:'',grant_id:'',grant_version:0},knowledge:[context]};
  expect(conversationKnowledgeUnion([message,assistant])).toEqual([pin]);
  expect(conversationKnowledgeUnion([assistant])).toEqual([pin]);
  expect(conversationKnowledgeUnion([message,{knowledge:[{...pin,revision:3}]}])).toBeUndefined();
  expect(validReplySource(source,'p','t','o')).toBe(true);
  expect(validReplySource({...source,messages:[assistant],coverage:{total:3,included:1,first_sequence:2,last_sequence:2,complete:false}},'p','t','o')).toBe(false);
  const {knowledge:_,...without}=source;expect(validReplySource(without,'p','t','o')).toBe(false);
  expect(validReplySource({...source,knowledge:[{...context,sha256:'b'.repeat(64)}]},'p','t','o')).toBe(false);
});
