import {loader} from '@monaco-editor/react';
import * as monaco from 'monaco-editor/esm/vs/editor/editor.api.js';
// Retain the full entry's editor features and all bundled language services.
import 'monaco-editor/esm/vs/editor/editor.main.js';
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker';
import TypeScriptWorker from 'monaco-editor/esm/vs/language/typescript/ts.worker?worker';
import JSONWorker from 'monaco-editor/esm/vs/language/json/json.worker?worker';
import CSSWorker from 'monaco-editor/esm/vs/language/css/css.worker?worker';
import HTMLWorker from 'monaco-editor/esm/vs/language/html/html.worker?worker';

// All editor assets and workers are bundled locally. Monaco's ESM instance has
// no public runtime NLS switch; the visible note states that precise boundary.
(self as any).MonacoEnvironment={getWorker(_:unknown,label:string){if(label==='typescript'||label==='javascript')return new TypeScriptWorker();if(label==='json')return new JSONWorker();if(['css','scss','less'].includes(label))return new CSSWorker();if(['html','handlebars','razor'].includes(label))return new HTMLWorker();return new EditorWorker();}};
loader.config({monaco});
export {monaco};
export const prepareMonaco=()=>loader.init();
