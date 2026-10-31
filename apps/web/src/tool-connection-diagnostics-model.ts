export type ToolConnectionDiagnosticAction='detect'|'configure'|'workspace'|'guide'|'reconcile';
export type ToolConnectionDiagnosticReason='workspace_required'|'not_installed'|'unsupported_launcher'|'invalid_package'|'node_unavailable'|'probe_failed'|'model_catalogue_unavailable'|'configuration_required'|'original_operation'|'authorization_required'|'binding_changed'|'origin_changed'|'credential_unrecoverable'|'renewal_blocked'|'unknown';
export type ToolConnectionDiagnosticDescriptor=Readonly<{reason:ToolConnectionDiagnosticReason;actions:readonly ToolConnectionDiagnosticAction[];rawCode?:string}>;

type Rule=Pick<ToolConnectionDiagnosticDescriptor,'reason'|'actions'>;
const original:Rule={reason:'original_operation',actions:['reconcile']};
const rules:Readonly<Record<string,Rule>>={
 workspace_required:{reason:'workspace_required',actions:['workspace']},
 needs_workspace:{reason:'workspace_required',actions:['workspace']},
 not_installed:{reason:'not_installed',actions:['guide','detect']},
 wrapper_only:{reason:'unsupported_launcher',actions:['guide','detect']},
 unsupported_launcher:{reason:'unsupported_launcher',actions:['guide','detect']},
 invalid_package:{reason:'invalid_package',actions:['guide','detect']},
 node_unavailable:{reason:'node_unavailable',actions:['guide','detect']},
 probe_failed:{reason:'probe_failed',actions:['detect','configure']},
 configuration_required:{reason:'configuration_required',actions:['configure']},
 prepared:original,issuance_unknown:original,renewal_unknown:original,stop_unconfirmed:original,reading:original,unknown:original,
 authorization_required:{reason:'authorization_required',actions:['reconcile']},
 binding_changed:{reason:'binding_changed',actions:['reconcile']},
 origin_changed:{reason:'origin_changed',actions:['reconcile']},
 credential_unrecoverable:{reason:'credential_unrecoverable',actions:['reconcile']},
 managed_runner_renewal_blocked:{reason:'renewal_blocked',actions:['reconcile']},
 codex_discovery_invalid:{reason:'configuration_required',actions:['configure']},
 codex_discovery_credential_unavailable:{reason:'configuration_required',actions:['configure']},
 codex_discovery_catalogue_unavailable:{reason:'model_catalogue_unavailable',actions:['configure']},
 codex_discovery_failed:{reason:'model_catalogue_unavailable',actions:['configure']},
 codex_discovery_installation_changed:{reason:'probe_failed',actions:['detect']},
 codex_discovery_unknown:original,codex_discovery_not_stopped:original,
};

/** Presentation only. A caller owns the current source, original operation and
 * concrete callbacks; an unfamiliar code never authorizes a guessed retry. */
export function toolConnectionDiagnostic(reasonCode?:string,originalOperationPending=false):ToolConnectionDiagnosticDescriptor|undefined{
 const rawCode=typeof reasonCode==='string'&&/^[a-z][a-z0-9_]{0,79}$/.test(reasonCode)?reasonCode:undefined;
 const source=rawCode?{rawCode}:{};
 if(originalOperationPending)return {...original,...source};
 if(reasonCode===undefined||reasonCode==='')return;
 return {...(rawCode&&Object.hasOwn(rules,rawCode)?rules[rawCode]:{reason:'unknown' as const,actions:[]}),...source};
}
