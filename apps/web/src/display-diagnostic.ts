import type { DisplayDiagnostic } from '../../../packages/contracts/src';
import { displaySourceDefinitions } from '../../../packages/contracts/src/display-sources';
import { catalogParameters } from './i18n/catalog';
import { formatServerCatalogMessage } from './i18n';

const plain = (value: unknown): value is Record<string, unknown> =>
  !!value && typeof value === 'object' && !Array.isArray(value)
  && (Object.getPrototypeOf(value) === Object.prototype || Object.getPrototypeOf(value) === null);

/** Display identity proves this complete raw reason only. It never grants eligibility. */
export function renderDisplayDiagnostic(source: string, diagnostic: unknown): {text:string;localized:boolean} {
  const fallback = {text:source,localized:false};
  if (!plain(diagnostic) || diagnostic.schema_version !== 1
    || Object.keys(diagnostic).some(key => !['schema_version','message_key','message_params'].includes(key))
    || typeof diagnostic.message_key !== 'string') return fallback;
  const identity = diagnostic as unknown as DisplayDiagnostic;
  const definition = displaySourceDefinitions.find(item => item.key === identity.message_key);
  if (!definition) return fallback;
  const parameters = definition.parameters.map(parameter => ({...parameter}));
  const values = catalogParameters({parameters}, identity.message_params);
  if (!values) return fallback;
  const canonical = definition.source_template.replace(/\{\{\.([a-z][a-z0-9_]*)\}\}/g,
    (_all, name: string) => String(values[name]));
  if (canonical !== source) return fallback;
  const text = formatServerCatalogMessage(definition.key, parameters, values);
  return text === undefined ? fallback : {text,localized:true};
}
