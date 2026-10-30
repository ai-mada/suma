// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
// Keep the actual installed React loader; only browser worker/ESM rendering
// boundaries are replaced. Its real init must resolve the configured instance
// without appending a remote loader script.
vi.mock('monaco-editor/esm/vs/editor/editor.api.js',()=>({editor:{}}));
vi.mock('monaco-editor/esm/vs/editor/editor.main.js',()=>({}));
vi.mock('monaco-editor/esm/vs/editor/editor.worker?worker',()=>({default:class {kind='editor';}}));
vi.mock('monaco-editor/esm/vs/language/typescript/ts.worker?worker',()=>({default:class {kind='typescript';}}));
vi.mock('monaco-editor/esm/vs/language/json/json.worker?worker',()=>({default:class {kind='json';}}));
vi.mock('monaco-editor/esm/vs/language/css/css.worker?worker',()=>({default:class {kind='css';}}));
vi.mock('monaco-editor/esm/vs/language/html/html.worker?worker',()=>({default:class {kind='html';}}));
afterEach(()=>{delete (globalThis as any).MonacoEnvironment;vi.restoreAllMocks();});
it('initializes the real shared loader from the skill entry and retains the complete local IDE worker route',async()=>{
 const sdk=await import('@monaco-editor/react'),configure=vi.spyOn(sdk.loader,'config');
 const compare=await import('./SkillCatalogRevisionEditor'),configured=await compare.prepareRevisionEditor();
 expect(document.querySelector('script[src]')).toBeNull();const route=(globalThis as any).MonacoEnvironment.getWorker;
 for(const [label,kind] of [['editorWorkerService','editor'],['typescript','typescript'],['javascript','typescript'],['json','json'],['scss','css'],['css','css'],['less','css'],['html','html'],['handlebars','html'],['razor','html']])expect(route('workerMain.js',label).kind).toBe(kind);
 const shared=await import('./monaco-local');expect(await shared.prepareMonaco()).toBe(configured);expect(configured).toBe(shared.monaco);expect((globalThis as any).MonacoEnvironment.getWorker).toBe(route);expect(configure).toHaveBeenCalledTimes(1);
});
