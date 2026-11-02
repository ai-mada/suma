'use strict';
const reasons=new Set(['native_version_unsupported','installation_changed','sdk_assets_missing','sdk_assets_invalid','node_runtime_unavailable','credential_reference_unavailable']);

/** Internal, finite preparation evidence. Only the reason crosses IPC; callers
 * must never publish Error.message, file names, environment values or stderr. */
class ToolPreparationError extends Error{
 constructor(reason,message='Tool preparation unavailable'){
  if(!reasons.has(reason))throw new TypeError('Invalid tool preparation reason');
  super(message);this.name='ToolPreparationError';this.reason=reason;
 }
}
const preparationReason=error=>error instanceof ToolPreparationError&&reasons.has(error.reason)?error.reason:undefined;

/** Use only around a concrete SDK file/JSON validation or native Node check.
 * Missing-file classification uses filesystem codes, never exception text.
 * Arbitrary controller/provider failures are deliberately not wrapped here. */
function preparationCheck(reason,check,missingReason=reason){
 try{return check();}catch(error){
  if(error instanceof ToolPreparationError)throw error;
  throw new ToolPreparationError(error?.code==='ENOENT'||error?.code==='ENOTDIR'?missingReason:reason);
 }
}
const sdkAssetRead=check=>preparationCheck('sdk_assets_invalid',check,'sdk_assets_missing');
const nodeRuntimeCheck=check=>preparationCheck('node_runtime_unavailable',check);
module.exports={ToolPreparationError,preparationReason,preparationCheck,sdkAssetRead,nodeRuntimeCheck};
