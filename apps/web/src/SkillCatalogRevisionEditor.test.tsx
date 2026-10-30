// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
import {loader} from '@monaco-editor/react';
import type {SkillCatalogDetail} from '../../../packages/contracts/src';
import SkillCatalogRevisionEditor from './SkillCatalogRevisionEditor';

// Retain the installed React DiffEditor and loader. Only Monaco's browser
// model/widget boundary is replaced so its real disposal order is exercised.
vi.mock('./monaco-local',()=>({prepareMonaco:()=>Promise.resolve(runtime)}));
type Uri={scheme:string;toString:()=>string};
type Model={uri:Uri;value:string;disposeCalls:number;disposed:boolean;dispose:()=>void;isDisposed:()=>boolean;setValue:(value:string)=>void};
type Pair={original:Model;modified:Model};
type Widget={getModel:()=>Pair|null;setModel:(pair:Pair|null)=>void;dispose:()=>void;updateOptions:()=>void;getOriginalEditor:()=>unknown;getModifiedEditor:()=>unknown};
const models=new Map<string,Model>(),allModels:Model[]=[],widgets:Widget[]=[],violations:string[]=[];
const parse=(value:string):Uri=>({scheme:value.split(':')[0],toString:()=>value});
function createModel(value:string,_language:string,uri:Uri){
 const model:Model={uri,value,disposed:false,disposeCalls:0,isDisposed:()=>model.disposed,setValue:value=>{model.value=value;},dispose:()=>{model.disposeCalls++;if(widgets.some(widget=>{const pair=widget.getModel();return pair?.original===model||pair?.modified===model;}))violations.push('TextModel got disposed before DiffEditorWidget model got reset');model.disposed=true;models.delete(uri.toString());}};
 models.set(uri.toString(),model);allModels.push(model);return model;
}
function createDiffEditor(){
 let pair:Pair|null=null;
 const side=(name:'original'|'modified')=>({getModel:()=>pair?.[name],setModel:(model:Model)=>{if(pair)pair[name]=model;},getOption:()=>true,getValue:()=>pair?.[name].value,setValue:(value:string)=>pair?.[name].setValue(value),executeEdits:()=>{},pushUndoStop:()=>{}});
 const widget:Widget={getModel:()=>pair,setModel:value=>{pair=value;},dispose:()=>{pair=null;},updateOptions:()=>{},getOriginalEditor:()=>side('original'),getModifiedEditor:()=>side('modified')};widgets.push(widget);return widget;
}
const runtime={Uri:{parse},editor:{createDiffEditor,createModel,getModel:(uri:Uri)=>models.get(uri.toString()),setTheme:()=>{},setModelLanguage:()=>{},EditorOption:{readOnly:0}}};
const revision=(version:number):SkillCatalogDetail=>({id:'shared-skill',version,state:'published',title:'Approved skill',summary:'',domain:'engineering',scenarios:['review'],tags:[],origin:{project_id:'source',knowledge_id:'skill',knowledge_version:version,revision:version,sha256:String(version).repeat(64)},source_current:true,eligibility:'current',created_by:'reviewer',created_at:'2026-10-30T00:00:00Z',reviewed_by:'reviewer',updated_at:'2026-10-30T00:00:00Z',reason:'Approved',can_manage:false,can_adopt:false,allowed_actions:[],content:'Fixed body '+version});
let root:Root|undefined,host:HTMLDivElement;
async function settle(){for(let n=0;n<5;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(right:number){await act(async()=>{root!.render(<SkillCatalogRevisionEditor key={'pair-'+right} left={revision(1)} right={revision(right)} scope="owned-skill-comparison" leftLabel="Left" rightLabel="Right" label="Comparison"/>);});await settle();}
afterEach(async()=>{for(const widget of widgets)widget.setModel(null);if(root)await act(async()=>root!.unmount());root=undefined;host?.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
it('detaches the installed DiffEditor before releasing owned models on pair change and SPA unmount, preserving the IDE model',async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);loader.config({monaco:runtime as any});host=document.createElement('div');document.body.append(host);root=createRoot(host);
 const ide=createModel('const answer: number = 42;','typescript',parse('file:///review-guide.ts'));
 await render(2);expect(widgets).toHaveLength(1);const first=widgets[0].getModel()!;expect(first).not.toBeNull();
 await render(3);expect(violations).toEqual([]);expect(first.original.isDisposed()).toBe(true);expect(first.modified.isDisposed()).toBe(true);expect(widgets).toHaveLength(2);expect(ide.isDisposed()).toBe(false);
 const second=widgets[1].getModel()!;await act(async()=>{root!.unmount();root=undefined;});expect(violations).toEqual([]);expect(second.original.isDisposed()).toBe(true);expect(second.modified.isDisposed()).toBe(true);expect(allModels.filter(model=>model!==ide).every(model=>model.disposeCalls===1)).toBe(true);expect(models.get(ide.uri.toString())).toBe(ide);expect(ide.disposeCalls).toBe(0);
});
