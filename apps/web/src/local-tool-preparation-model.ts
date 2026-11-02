import {preparationForLocalTool,type ToolPreparationTarget} from './tool-preparation-target';
import {validCommandCompositionOutput} from './command-composition-model';
import {runnerSetupLiteral} from './cursor-setup-guide-model';

export type LocalToolPreparationTarget=Extract<ToolPreparationTarget,{special:false}>;
export type LocalToolPreparationDraft={
  shell:'powershell'|'bash';
  commands:string;
  localTool:LocalToolPreparationTarget['localTool'];
};

/** Reuse the one protocol directory. A native product name must not select a
 * different adapter, or bypass the explicit inputs of an SDK/ACP preparer. */
export function localToolPreparationTarget(value:unknown):LocalToolPreparationTarget|undefined{
  const target=preparationForLocalTool(value);
  return target?.special===false?target:undefined;
}

/** This closed reference says where the user intends to create a file. It
 * proves neither installation nor file creation; the original native writer
 * remains responsible for actual paths, links, cancellation and no-overwrite. */
export function validLocalToolPreparationDraft(value:unknown):value is LocalToolPreparationDraft{
  if(!value||typeof value!=='object'||Array.isArray(value)||![Object.prototype,null].includes(Object.getPrototypeOf(value)))return false;
  const row=value as Record<string,unknown>;
  return Reflect.ownKeys(row).length===3&&['shell','commands','localTool'].every(key=>Object.hasOwn(row,key))&&
    (row.shell==='powershell'||row.shell==='bash')&&validCommandCompositionOutput(row.commands)&&!!localToolPreparationTarget(row.localTool);
}

/** Fixed existing CLI mode only. Shell literals are shared with the mature
 * explicit preparers; no control address, identity, environment or task flags
 * are introduced by preparing or copying this local command. */
export function localToolPreparationCommand(value:unknown):{prepare:string}|undefined{
  if(!validLocalToolPreparationDraft(value))return;
  return {prepare:'suma-runner --prepare-tools '+runnerSetupLiteral(value.commands,value.shell)+' --local-tool '+runnerSetupLiteral(value.localTool,value.shell)};
}
