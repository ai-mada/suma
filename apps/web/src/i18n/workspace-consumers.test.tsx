// @vitest-environment jsdom
import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { IDE } from "../IDE";
import { Tokens } from "../Tokens";
import { ExecutionWorkspace } from "../ExecutionWorkspace";
import { ErrorBoundary, ErrorRecovery } from "../ErrorBoundary";
import { EntityPager } from "../pagination";
import { TokenCreation } from "../token-requests";
import { InvitationAcceptanceRequest } from "../invitation-request";
import { ErrorNotice } from "../ui";
import { renderDiagnostic, setLocale, setServerCatalog, useLocale } from "./index";

// Monaco's canvas and workers need browser geometry. Only that external
// rendering boundary is replaced; file state, controls and requests stay real.
vi.mock("@monaco-editor/react", () => ({ loader: {config: vi.fn()}, default: (props: {value: string; onChange: (value:string)=>void; options?:{ariaLabel?:string}}) => <textarea aria-label={props.options?.ariaLabel || "editor fixture"} value={props.value} onChange={event=>props.onChange(event.target.value)}/> }));
vi.mock("monaco-editor/esm/vs/editor/editor.main.js", () => ({}));
vi.mock("monaco-editor/esm/vs/editor/editor.api.js", () => ({}));
vi.mock("monaco-editor/esm/vs/editor/editor.worker?worker", () => ({default: class {}}));
vi.mock("monaco-editor/esm/vs/language/typescript/ts.worker?worker", () => ({default: class {}}));
vi.mock("monaco-editor/esm/vs/language/json/json.worker?worker", () => ({default: class {}}));
vi.mock("monaco-editor/esm/vs/language/css/css.worker?worker", () => ({default: class {}}));
vi.mock("monaco-editor/esm/vs/language/html/html.worker?worker", () => ({default: class {}}));
const project="original-project", original={path:"src/用户原件.ts",content:"// 原始文件 <strong>",hash:"original-file-hash",language:"typescript"};
let host:HTMLDivElement, root:Root|undefined;
let reads:string[], writes:{path:string;body:Record<string,unknown>}[], fixture:(path:string,body?:Record<string,unknown>,params?:URLSearchParams)=>Response|Promise<Response>;
const json=(value:unknown,status=200)=>new Response(JSON.stringify(value),{status,headers:{"Content-Type":"application/json"}});
beforeEach(async()=>{
 localStorage.clear();sessionStorage.clear();reads=[];writes=[];await setLocale("en-US",{persist:false});
 vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT",true);host=document.createElement("div");document.body.append(host);
 fixture=async(path,body,params)=>path==="/workspace"?json({configured:true,binding_available:true,name:"用户目录原名"}):path==="/workspace/tree"?json({project_id:project,directory:params?.get("directory")||"",entries:params?.get("directory")==="src"?[{path:original.path,name:"用户原件.ts",kind:"file"}]:[{path:"src",name:"src",kind:"directory"}],limit:100,next_cursor:"",snapshot_sha256:"a".repeat(64),observed_at:"2026-10-28T00:00:00Z"}):path==="/workspace/file"?json(body?{...original,content:body.content,hash:"saved-hash"}:original):json({items:[],total:0});
 vi.stubGlobal("fetch",vi.fn(async(input:string,options?:RequestInit)=>{const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,"");const body=options?.body?JSON.parse(String(options.body)):undefined;if(body)writes.push({path,body});else reads.push(path+url.search);return fixture(path,body,url.searchParams);}));
});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale("zh-CN",{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let n=0;n<4;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(node:ReactNode){await act(async()=>{root=createRoot(host);root.render(node);});await settle();}
async function locale(value:"en-US"|"zh-CN"){await act(async()=>{await setLocale(value,{persist:false});});await settle();}
async function click(text:string){const button=[...host.querySelectorAll<HTMLButtonElement>("button")].find(item=>item.textContent?.trim()===text);expect(button,"button "+text).toBeDefined();await act(async()=>button!.click());await settle();}
async function value(input:HTMLInputElement|HTMLSelectElement|HTMLTextAreaElement,text:string){const prototype=input instanceof HTMLSelectElement?HTMLSelectElement.prototype:input instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(prototype,"value")!.set!.call(input,text);input.dispatchEvent(new Event(input instanceof HTMLSelectElement?"change":"input",{bubbles:true}));});}
async function submit(){await act(async()=>{host.querySelector("form")!.dispatchEvent(new Event("submit",{bubbles:true,cancelable:true}));});await settle();}

it("relocalizes IDE controls and dirty navigation refusal without reloading the original file",async()=>{
 await render(<IDE project={project}/>);expect(host.textContent).toContain("Edit files in the bound project directly.");await click("src");await click("用户原件.ts");const editor=host.querySelector<HTMLTextAreaElement>('textarea[aria-label="Project file editor"]')!;expect(editor).not.toBeNull();await value(editor,"// 用户草稿 <strong> {{raw}}");
 const guard=new Event("suma:navigate",{cancelable:true});await act(async()=>{window.dispatchEvent(guard);});expect(guard.defaultPrevented).toBe(true);expect(host.textContent).toContain("This file has unsaved changes.");const count=reads.length;
 await locale("zh-CN");expect(editor.isConnected).toBe(true);expect(editor.value).toBe("// 用户草稿 <strong> {{raw}}");expect(editor.getAttribute("aria-label")).toBe("项目文件编辑器");expect(host.textContent).toContain("当前文件有未保存修改");expect(reads).toHaveLength(count);expect(writes).toHaveLength(0);
});
it("keeps the original unknown file-save operation, hash and submitted body across language changes and later edits",async()=>{
 const base=fixture;let failed=false;fixture=async(path,body,params)=>{if(path==="/workspace/file"&&body&&!failed){failed=true;throw new TypeError("lost receipt");}return base(path,body,params);};
 await render(<IDE project={project}/>);await click("src");await click("用户原件.ts");const editor=host.querySelector<HTMLTextAreaElement>("textarea")!;await value(editor,"// submitted original");await click("Save");expect(writes).toHaveLength(1);const envelope=structuredClone(writes[0].body);await value(editor,"// newer 用户草稿");const count=reads.length;await locale("zh-CN");expect(host.textContent).toContain("恢复上次保存");expect(writes).toHaveLength(1);expect(reads).toHaveLength(count);await click("恢复上次保存");expect(writes[1].body).toEqual(envelope);expect(envelope).toMatchObject({project_id:project,path:original.path,expected_hash:original.hash,content:"// submitted original"});expect(editor.value).toBe("// newer 用户草稿");
});
it("keeps reconciliation evidence raw while translating recovery without issuing another file save",async()=>{
 const base=fixture;fixture=async(path,body,params)=>path==="/workspace/file"&&body?json({error:{code:"workspace_reconciliation_required",message:"原始后端原因 <strong>",details:{operation_id:body.idempotency_key,path:original.path}}},409):base(path,body,params);
 await render(<IDE project={project}/>);await click("src");await click("用户原件.ts");await value(host.querySelector<HTMLTextAreaElement>("textarea")!,"// raw submission");await click("Save");expect(host.textContent).toContain("The previous save needs its actual result verified");expect(host.textContent).toContain(original.content);expect(host.textContent).toContain("原始后端原因 <strong>");await locale("zh-CN");expect(host.textContent).toContain("上次保存需要核实真实结果");expect(host.textContent).toContain(original.hash);expect(host.textContent).toContain(writes[0].body.idempotency_key);expect(writes).toHaveLength(1);
});
it("retains a one-time token and its machine scope when switching language",async()=>{
 fixture=async(_path,body)=>json({token:"secret/raw/<never-translate>",expires_at:"2026-10-25T08:00:00Z",label:body?.label,scope:body?.scope,project_id:body?.project_id});
 await render(<Tokens project={project} projectName="原始项目名" admin/>);expect(host.textContent).toContain("Create a project access token");await value(host.querySelector<HTMLInputElement>('input[name="label"]')!,"原始令牌备注");await value(host.querySelector<HTMLSelectElement>("select")!,"agent");await submit();expect(writes[0].body).toMatchObject({project_id:project,scope:"agent",label:"原始令牌备注"});expect(host.querySelector(".token-result code")?.textContent).toBe("secret/raw/<never-translate>");await locale("zh-CN");expect(host.querySelector(".token-result code")?.textContent).toBe("secret/raw/<never-translate>");expect(host.querySelector(".token-result")?.textContent).toContain("Agent 工具调用");expect(writes).toHaveLength(1);
});
it("keeps a token's unknown receipt and original scope across translation and remount without recreating its secret",async()=>{
 fixture=async()=>{throw new TypeError("lost response");};await render(<Tokens project={project} projectName="原始项目名" admin/>);await value(host.querySelector<HTMLInputElement>('input[name="label"]')!,"原始未知令牌");await value(host.querySelector<HTMLSelectElement>("select")!,"api");await submit();expect(host.textContent).toContain("The creation receipt is unknown");const operation=writes[0].body.idempotency_key;await locale("zh-CN");expect(host.textContent).toContain("创建回执未知");expect(host.textContent).toContain(operation);expect(host.textContent).toContain("项目 API");await act(async()=>{root?.unmount();root=undefined;});await render(<Tokens project={project} projectName="原始项目名" admin/>);expect(host.textContent).toContain(operation);expect(host.textContent).toContain("项目 API");expect(host.querySelector<HTMLButtonElement>('form button')?.disabled).toBe(true);expect(writes).toHaveLength(1);expect(host.querySelector(".token-result")).toBeNull();
});
it("translates the queue and layout ARIA while retaining raw blockers, exact run links and request count",async()=>{
 fixture=async()=>json({items:[{run_id:"run/raw",node_id:"node/raw",node_name:"用户节点原名",workflow_name:"用户流程原名",type:"command",ready_at:"2026-10-24T00:00:00Z",attempt:1234,admission:{run_id:"run/raw",node_id:"node/raw",run_version:1,status:"blocked",blockers:[{code:"vendor_custom",message:"原始供应商原因 <strong>"}],requests:[],reservations:[],resources:[],as_of:"2026-10-24T00:00:00Z"}}],total:2345});const onRun=vi.fn();await render(<ExecutionWorkspace project={project} refresh={0} layout="queue" onLayout={()=>{}} onRun={onRun}><p>list</p></ExecutionWorkspace>);expect(host.querySelector('[aria-label="Execution record view"]')).not.toBeNull();expect(host.textContent).toContain("Ready queue");expect(host.textContent).toContain("1,234");expect(host.textContent).toContain("2,345");expect(host.textContent).toContain("原始供应商原因 <strong>");const link=host.querySelector<HTMLAnchorElement>(".scheduling-open-link")!,url=link.href,count=reads.length;await locale("zh-CN");expect(link.isConnected).toBe(true);expect(link.href).toBe(url);expect(host.textContent).toContain("就绪队列");expect(host.textContent).toContain("原始供应商原因 <strong>");expect(reads).toHaveLength(count);await act(async()=>link.click());expect(onRun).toHaveBeenCalledWith("run/raw","node/raw");
});
it("retranslates a captured boundary failure while keeping its original stack and refusing automatic recovery",async()=>{
 const error=new Error("原始异常 <script>");error.stack="Error: 原始异常 <script>\n    at original.ts:42";let attempts=0;function Broken():ReactNode{attempts++;throw error;}
 vi.spyOn(console,"error").mockImplementation(()=>{});await render(<ErrorBoundary><Broken/></ErrorBoundary>);expect(host.textContent).toContain("This page cannot be displayed right now");expect(host.querySelector("pre")?.textContent).toBe(error.stack);const attemptsAtFailure=attempts;await locale("zh-CN");expect(host.textContent).toContain("页面暂时无法显示");expect(host.querySelector("pre")?.textContent).toBe(error.stack);expect(attempts).toBe(attemptsAtFailure);expect(reads).toHaveLength(0);expect(writes).toHaveLength(0);
});
it("localizes chunk recovery actions and retains original project and page recovery URLs",async()=>{
 history.replaceState(null,"","?project=original-project&view=development&sub=runs&run=original-run&node=original-node");await render(<ErrorRecovery error={new Error("Failed to fetch dynamically imported module: /old-chunk.js")}/>);expect(host.textContent).toContain("Page resources have changed");const current=host.querySelector(".recovery-url")?.textContent,home=host.querySelector<HTMLAnchorElement>(".recovery-link")?.href;expect(home).toContain("project=original-project");expect(home).toContain("view=overview");expect(home).not.toContain("run=original-run");await locale("zh-CN");expect(host.textContent).toContain("页面资源已更新");expect(host.querySelector(".recovery-url")?.textContent).toBe(current);expect(host.querySelector<HTMLAnchorElement>(".recovery-link")?.href).toBe(home);history.replaceState(null,"","/");
});
it("relocalizes pager validation through a real consumer without rereading or losing the last successful window",async()=>{
 let invalid=false;fixture=async()=>invalid?json({items:null,total:8}):json({items:[{id:"original-record",title:"用户原始记录"}],total:8,next_cursor:"original-cursor"});const pager=new EntityPager(project,"tasks");await pager.first();invalid=true;await pager.more();function Snapshot(){useLocale();const page=pager.snapshot();return <div><p role="alert">{page.error}</p>{page.items?.map(item=><p key={item.id}>{item.title}</p>)}<code>{page.next_cursor}</code></div>;}
 await render(<Snapshot/>);expect(host.textContent).toContain("The list response has no valid records or total.");expect(host.textContent).toContain("用户原始记录");const count=reads.length;await locale("zh-CN");expect(host.textContent).toContain("列表响应缺少有效的记录或总数。");expect(host.textContent).toContain("original-cursor");expect(reads).toHaveLength(count);
});
it("does not read or create credentials for an unselected project and translates the guidance",async()=>{
 const pager=new EntityPager("","tasks");expect(await pager.first()).toBe(false);await render(<><IDE project=""/><Tokens project="" projectName="" admin/><ExecutionWorkspace project="" refresh={0} layout="queue" onLayout={()=>{}} onRun={()=>{}}>list</ExecutionWorkspace></>);expect(host.textContent).toContain("Select a project first");expect(host.querySelector("form")).toBeNull();expect(reads).toHaveLength(0);expect(writes).toHaveLength(0);await locale("zh-CN");expect(host.textContent).toContain("请先选择项目");expect(reads).toHaveLength(0);
});
it("keeps local one-time request refusal diagnostics translatable without exposing or replaying secrets",async()=>{
 fixture=async()=>{throw new TypeError("lost");};const token=new TokenCreation(),invitation=new InvitationAcceptanceRequest();await expect(token.create(project,"原始备注","agent")).rejects.toMatchObject({status:0});await expect(invitation.accept({token:"one-use-raw",email:"original@example.test",name:"原始用户",password:"raw-proof"})).rejects.toMatchObject({status:0});let tokenError:unknown,invitationError:unknown;try{await token.create("other-project","changed","runner");}catch(error){tokenError=error;}try{await invitation.accept({token:"changed",email:"changed@example.test",name:"Changed",password:"changed"});}catch(error){invitationError=error;}function Diagnostics(){useLocale();return <><p>{renderDiagnostic(tokenError)}</p><p>{renderDiagnostic(invitationError)}</p></>;}
 await render(<Diagnostics/>);expect(host.textContent).toContain("The credential creation result has not been confirmed");expect(host.textContent).toContain("The acceptance result has not been confirmed");await locale("zh-CN");expect(host.textContent).toContain("凭证创建结果尚未确认");expect(host.textContent).toContain("接受结果尚未确认");expect(writes).toHaveLength(2);expect(writes[0].body).toMatchObject({project_id:project,scope:"agent",label:"原始备注"});expect(writes[1].body).toMatchObject({token:"one-use-raw",email:"original@example.test",name:"原始用户",password:"raw-proof"});
});
it("restores the unknown token's original scope into the disabled form and its connection guidance",async()=>{
 sessionStorage.setItem("suma-token-unknown:"+project,JSON.stringify({operation_id:"original-unknown-operation",scope:"agent",label:"原始未知备注"}));await render(<Tokens project={project} projectName="原始项目" admin/>);expect(host.querySelector<HTMLSelectElement>("select")?.value).toBe("agent");expect(host.querySelector(".detail-section")?.textContent).toContain("Use an Agent-scoped token");await locale("zh-CN");expect(host.querySelector(".detail-section")?.textContent).toContain("使用 Agent 范围令牌");expect(writes).toHaveLength(0);
});
it("retains a typed server diagnostic in a captured paging snapshot so its real error notice can retranslate parameters",async()=>{
 setServerCatalog({version:"workspace-consumer",template_format:"named-dot-v1",default_locale:"zh-CN",supported_locales:["zh-CN","en-US"],messages:[{key:"workspace_fixture",parameters:[{name:"limit",kind:"integer"}],texts:{"zh-CN":"读取上限 {{.limit}}","en-US":"Read limit {{.limit}}"}}]});
 fixture=async()=>json({error:{code:"limit",message:"Read limit 7",message_key:"workspace_fixture",message_params:{limit:7}}},422);const pager=new EntityPager(project,"tasks");await pager.first();const page=pager.snapshot();function FailedPage(){useLocale();return <ErrorNotice error={page.failure||page.error}/>;}
 await render(<FailedPage/>);expect(host.textContent).toContain("Read limit 7");await locale("zh-CN");expect(host.textContent).toContain("读取上限 7");expect(reads).toHaveLength(1);
});
it("does not present a previous one-time secret as the result of a later unknown creation",async()=>{
 let requests=0;fixture=async(path,body)=>{if(path!=="/auth/tokens")return json({items:[],total:0});if(++requests>1)throw new TypeError("lost later receipt");return json({token:"first-secret-only",expires_at:"2026-10-25T08:00:00Z",scope:body?.scope,project_id:body?.project_id,label:body?.label});};await render(<Tokens project={project} projectName="原始项目" admin/>);await value(host.querySelector<HTMLInputElement>('input[name="label"]')!,"first original label");await submit();expect(host.querySelector(".token-result code")?.textContent).toBe("first-secret-only");await value(host.querySelector<HTMLInputElement>('input[name="label"]')!,"second original label");await submit();expect(host.textContent).toContain("The creation receipt is unknown");expect(host.querySelector(".token-result")).toBeNull();expect(writes).toHaveLength(2);
});
it("keeps the original unknown operation blocked when browser receipt storage fails and translates the actual recovery limitation",async()=>{
 vi.spyOn(Storage.prototype,"setItem").mockImplementation(()=>{throw new Error("storage unavailable");});fixture=async()=>{throw new TypeError("lost receipt");};await render(<Tokens project={project} projectName="原始项目" admin/>);await value(host.querySelector<HTMLInputElement>('input[name="label"]')!,"raw failed storage");await submit();expect(host.textContent).toContain("Browser storage is unavailable");expect(host.textContent).toContain(writes[0].body.idempotency_key);await locale("zh-CN");expect(host.textContent).toContain("浏览器存储不可用");expect(host.querySelector<HTMLButtonElement>('form button')?.disabled).toBe(true);expect(writes).toHaveLength(1);expect(host.querySelector(".token-result")).toBeNull();
});
