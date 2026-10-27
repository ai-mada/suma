import type {editor,IDisposable} from 'monaco-editor';

/** Current render facts; the callback remains the sole owner of save identity. */
export type WorkspaceSaveShortcutState={
 hasFile:boolean;
 dirty:boolean;
 pending:boolean;
 busy:boolean;
 conflict:boolean;
 save:()=>void|Promise<void>;
};
type ShortcutKeys={KeyMod:{readonly CtrlCmd:number};KeyCode:{readonly KeyS:number}};

/**
 * Register through Monaco's supported action service rather than a window
 * bubble listener: Monaco may consume the original keyboard event itself.
 * CtrlCmd maps to Control on Windows/Linux and Command on macOS. With no
 * additional modifier bits it does not steal Save As or Alt-modified chords.
 *
 * The binding stays active for clean/busy/conflicted files so the editor owns
 * the chord without falling through to the browser's Save Page dialog. Those
 * states produce no mutation. A pending request takes precedence over dirty:
 * the existing save function must recover that exact original body and caller,
 * even when newer typing has returned to the previous displayed content.
 *
 * `current` must read the latest render ref. This adapter never creates an
 * operation, retries automatically, changes a file, or clears uncertain state.
 */
export function bindWorkspaceSaveShortcut(
 target:Pick<editor.IStandaloneCodeEditor,'addAction'>,
 keys:ShortcutKeys,
 current:()=>WorkspaceSaveShortcutState,
 label:string,
):IDisposable{
 let active=true,inFlight=false;
 const action=target.addAction({
  id:'suma.workspace.save',label,
  keybindings:[keys.KeyMod.CtrlCmd|keys.KeyCode.KeyS],
  run:async()=>{
   if(!active||inFlight)return;
   const state=current();
   if(!state.hasFile||state.busy||state.conflict||(!state.pending&&!state.dirty))return;
   // React's busy update is asynchronous. A second keydown in the same turn
   // must not enter save again before that next render has reached `current`.
   inFlight=true;
   try{await state.save();}finally{inFlight=false;}
  },
 });
 return {dispose(){if(active){active=false;action.dispose();}}};
}
