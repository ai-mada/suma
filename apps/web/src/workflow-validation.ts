import type { WorkflowValidation } from '../../../packages/contracts/src';

/** Display projection only. Keys and params enable catalog translation, while
 * every fallback retains the corresponding raw compiler error as its source. */
export function workflowValidationMessages(result: WorkflowValidation): unknown[] {
  const errors = Array.isArray(result.errors) ? result.errors : [];
  if (!Array.isArray(result.diagnostics) || result.diagnostics.length !== errors.length)
    return errors;
  return errors.map((source,index)=> {
    const diagnostic=result.diagnostics[index];
    return diagnostic && typeof diagnostic.message === 'string' && diagnostic.message &&
      typeof diagnostic.message_key === 'string' && diagnostic.localized === true
      ? {...diagnostic,message:source} : source;
  });
}
