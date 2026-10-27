import type {editor} from 'monaco-editor';
import {expect,it,vi} from 'vitest';
import {bindWorkspaceSaveShortcut,type WorkspaceSaveShortcutState} from './workspace-save-shortcut';

const keys={KeyMod:{CtrlCmd:2048},KeyCode:{KeyS:49}};
function editorAction(){
 let action:editor.IActionDescriptor|undefined;
 const dispose=vi.fn();
 const instance={addAction:(next:editor.IActionDescriptor)=>{action=next;return {dispose};}};
 // The action intentionally never reads Monaco's command editor argument.
 return {instance,dispose,action:()=>action!,run:()=>action!.run(instance as unknown as editor.ICodeEditor)};
}

it('registers a disposable platform save action and reads current blockers instead of a stale render',async()=>{
 const target=editorAction(),save=vi.fn();
 let state:WorkspaceSaveShortcutState={hasFile:true,dirty:true,pending:false,busy:false,conflict:false,save};
 const binding=bindWorkspaceSaveShortcut(target.instance,keys,()=>state,'Save');
 expect(target.action()).toMatchObject({id:'suma.workspace.save',label:'Save',keybindings:[2048|49]});
 for(const override of [{hasFile:false},{busy:true},{conflict:true},{dirty:false}]){
  state={hasFile:true,dirty:true,pending:false,busy:false,conflict:false,save,...override};
  await target.run();
 }
 expect(save).not.toHaveBeenCalled();
 // Even a clean visible draft may have an unresolved original operation. The
 // existing save callback, not this adapter, owns its envelope/caller/key.
 state={...state,dirty:false,pending:true};
 await target.run();expect(save).toHaveBeenCalledTimes(1);
 binding.dispose();binding.dispose();expect(target.dispose).toHaveBeenCalledTimes(1);
 await target.run();expect(save).toHaveBeenCalledTimes(1);
});

it('admits one save until its receipt settles and never substitutes a new callback while it is in flight',async()=>{
 const target=editorAction();let release:()=>void=()=>{};
 const original=vi.fn(()=>new Promise<void>(resolve=>{release=resolve;})),next=vi.fn();
 let state:WorkspaceSaveShortcutState={hasFile:true,dirty:true,pending:false,busy:false,conflict:false,save:original};
 bindWorkspaceSaveShortcut(target.instance,keys,()=>state,'Save');
 const attempt=target.run();
 state={...state,save:next,pending:true};
 await target.run();expect(original).toHaveBeenCalledTimes(1);expect(next).not.toHaveBeenCalled();
 release();await attempt;await target.run();expect(next).toHaveBeenCalledTimes(1);
});
