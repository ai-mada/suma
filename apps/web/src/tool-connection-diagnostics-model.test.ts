import {expect,it} from 'vitest';
import {toolConnectionDiagnostic} from './tool-connection-diagnostics-model';

it('T203 routes exact SDK preparation causes only to existing callbacks and always prioritizes the original operation',()=>{
 const rules={native_version_unsupported:['guide','detect'],installation_changed:['detect','guide'],sdk_assets_missing:['guide'],sdk_assets_invalid:['guide'],node_runtime_unavailable:['guide'],credential_reference_unavailable:['configure']};
 for(const [reason,actions] of Object.entries(rules)){
  expect(toolConnectionDiagnostic(reason)).toEqual({reason,actions,rawCode:reason});
  expect(toolConnectionDiagnostic(reason,true)).toEqual({reason:'original_operation',actions:['reconcile'],rawCode:reason});
 }
 expect(toolConnectionDiagnostic('unavailable')).toEqual({reason:'unknown',actions:[],rawCode:'unavailable'});
 expect(toolConnectionDiagnostic('sdk_assets_future')).toEqual({reason:'unknown',actions:[],rawCode:'sdk_assets_future'});
});

it('limits an unresolved original operation to reconciliation even when a new setup action would otherwise be available',()=>{
 for(const reason of ['workspace_required','not_installed','node_unavailable','configuration_required','probe_failed','future_tool_reason',undefined]){
  const value=toolConnectionDiagnostic(reason,true);
  expect(value?.reason,reason).toBe('original_operation');
  expect(value?.actions,reason).toEqual(['reconcile']);
 }
 for(const reason of ['prepared','issuance_unknown','renewal_unknown','stop_unconfirmed','reading','unknown'])expect(toolConnectionDiagnostic(reason)?.actions,reason).toEqual(['reconcile']);
 expect(toolConnectionDiagnostic('renewal_future')).toEqual({reason:'unknown',actions:[],rawCode:'renewal_future'});
});

it('maps current safe causes to explicit existing actions without guessing a retry for an unknown code',()=>{
 expect(toolConnectionDiagnostic('managed_runner_renewal_blocked')).toEqual({reason:'renewal_blocked',actions:['reconcile'],rawCode:'managed_runner_renewal_blocked'});
 expect(toolConnectionDiagnostic('managed_runner_renewal_blocked',true)).toEqual({reason:'original_operation',actions:['reconcile'],rawCode:'managed_runner_renewal_blocked'});
 for(const reason of ['workspace_required','needs_workspace'])expect(toolConnectionDiagnostic(reason)?.actions,reason).toEqual(['workspace']);
 for(const reason of ['not_installed','wrapper_only','unsupported_launcher','invalid_package','node_unavailable'])expect(toolConnectionDiagnostic(reason)?.actions,reason).toEqual(['guide','detect']);
 expect(toolConnectionDiagnostic('configuration_required')?.actions).toEqual(['configure']);
 expect(toolConnectionDiagnostic('probe_failed')?.actions).toEqual(['detect','configure']);
 for(const reason of ['authorization_required','binding_changed','origin_changed','credential_unrecoverable'])expect(toolConnectionDiagnostic(reason)?.actions,reason).toEqual(['reconcile']);
 const codexCauses={codex_discovery_invalid:['configure'],codex_discovery_credential_unavailable:['configure'],codex_discovery_catalogue_unavailable:['configure'],codex_discovery_failed:['configure'],codex_discovery_installation_changed:['detect'],codex_discovery_unknown:['reconcile'],codex_discovery_not_stopped:['reconcile']};
 for(const [reason,actions] of Object.entries(codexCauses)){
  expect(toolConnectionDiagnostic(reason)?.actions,reason).toEqual(actions);
  expect(toolConnectionDiagnostic(reason,true)?.actions,reason).toEqual(['reconcile']);
 }
 expect(toolConnectionDiagnostic('future_tool_reason')).toEqual({reason:'unknown',actions:[],rawCode:'future_tool_reason'});
 for(const reason of ['secret=value','unexpected\ncode','x'.repeat(81)])expect(toolConnectionDiagnostic(reason)).toEqual({reason:'unknown',actions:[]});
 expect(toolConnectionDiagnostic()).toBeUndefined();
});
