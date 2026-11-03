import type {ToolInstallationProbeDiagnostic} from '../../../packages/contracts/src';

export type {ToolInstallationProbeDiagnostic};
const rules:Readonly<Record<string,{stages:readonly string[];action:string}>>={
 source_unavailable:{stages:['resolve','source'],action:'guide'},
 source_changed:{stages:['source'],action:'scan'},
 configuration_invalid:{stages:['source'],action:'guide'},
 permission_denied:{stages:['resolve','source','version','help','protocol'],action:'guide'},
 version_unsupported:{stages:['version'],action:'guide'},
 launch_failed:{stages:['version','help','protocol'],action:'guide'},
 probe_failed:{stages:['version','help','protocol'],action:'guide'},
 probe_timeout:{stages:['version','help','protocol'],action:'scan'},
 output_limit_exceeded:{stages:['version','help','protocol'],action:'guide'},
 help_contract_mismatch:{stages:['help'],action:'guide'},
 protocol_mismatch:{stages:['protocol'],action:'guide'},
 stop_unverified:{stages:['version','help','protocol'],action:'none'},
};
const record=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));

/** Fixed observed facts only. Neither a diagnostic nor its suggested action
 * grants permission to run a probe, prepare configuration or enable a tool. */
export function validToolInstallationProbeDiagnostic(value:unknown):value is ToolInstallationProbeDiagnostic{
 if(!record(value)||Reflect.ownKeys(value).length!==3||!['stage','code','next_action'].every(key=>Object.hasOwn(value,key))||typeof value.code!=='string'||!Object.hasOwn(rules,value.code))return false;
 const rule=rules[value.code];
 return rule.stages.includes(value.stage as string)&&value.next_action===rule.action;
}

/** An absent field is not an observed success. An explicit null, undefined or
 * contradictory success row is rejected by both installation readers. */
export function validToolInstallationProbeDiagnosticField(value:Record<string,unknown>):boolean{
 if(!Object.hasOwn(value,'diagnostic'))return true;
 return value.launch_supported===false&&!['not_installed','workspace_required','configuration_required','acp_configuration_required','cursor_editor_only'].includes(String(value.reason_code))&&validToolInstallationProbeDiagnostic(value.diagnostic)&&(value.reason_code!=='scan_incomplete'||value.diagnostic.stage==='resolve');
}

export function hasUnverifiedInstallationStop(items:readonly {diagnostic?:ToolInstallationProbeDiagnostic}[]):boolean{
 return items.some(item=>item.diagnostic?.code==='stop_unverified');
}
