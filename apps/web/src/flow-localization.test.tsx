// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {beforeEach,afterEach,expect,it,vi} from 'vitest';
import {WorkflowView} from './Workflow';
import {setLocale} from './i18n';
import {ReactFlow,Controls,MiniMap,Position,type Node} from '@xyflow/react';
import {flowAriaLabelConfig,flowHandleLabel} from './flow-localization';
import {useLocale} from './i18n';

let host:HTMLDivElement,root:Root;
beforeEach(async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 vi.stubGlobal('ResizeObserver',class {observe(){} unobserve(){} disconnect(){}});
 vi.stubGlobal('DOMMatrixReadOnly',class {m22=1});
 vi.stubGlobal('fetch',vi.fn(async()=>new Response(JSON.stringify({items:[],total:0}),{headers:{'Content-Type':'application/json'}})));
 await setLocale('zh-CN',{persist:false});host=document.createElement('div');document.body.append(host);root=createRoot(host);
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
it('localizes real official Controls titles and ARIA in the actual Workflow without extra reads',async()=>{
 await act(async()=>root.render(<WorkflowView project="p" refresh={0} onRefresh={()=>{}} onRun={()=>{}} onNode={()=>{}}/>));
 const zoom=host.querySelector<HTMLButtonElement>('.react-flow__controls-zoomin')!;
 expect(zoom).not.toBeNull();expect(zoom.title).toBe('放大');expect(zoom.getAttribute('aria-label')).toBe('放大');
 const count=vi.mocked(fetch).mock.calls.length;zoom.focus();
 await act(async()=>{await setLocale('en-US',{persist:false});});
 expect(host.querySelector('.react-flow__controls-zoomin')).toBe(zoom);expect(document.activeElement).toBe(zoom);
 expect(zoom.title).toBe('Zoom in');expect(zoom.getAttribute('aria-label')).toBe('Zoom in');expect(vi.mocked(fetch).mock.calls).toHaveLength(count);
 expect(host.querySelector('.react-flow__attribution a')?.getAttribute('href')).toContain('reactflow.dev');
});
it('keeps real node keyboard selection, movement, descriptions, dirty draft and viewport across locale',async()=>{
 await act(async()=>root.render(<WorkflowView project="p" refresh={0} onRefresh={()=>{}} onRun={()=>{}} onNode={()=>{}}/>));
 const add=[...host.querySelectorAll('button')].find(button=>button.textContent==='添加第一个节点')!;
 expect(add).toBeTruthy();await act(async()=>add.click());
 const node=host.querySelector<HTMLElement>('.react-flow__node')!;
 expect(node).not.toBeNull();expect(node.getAttribute('aria-roledescription')).toBe('节点');
 const description=()=>document.getElementById(node.getAttribute('aria-describedby')!)?.textContent;
 expect(description()).toContain('方向键');
 node.focus();await act(async()=>node.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true})));
 expect(node.classList.contains('selected')).toBe(true);
 await act(async()=>node.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true})));
 expect(host.querySelector('[aria-live="assertive"]')?.textContent).toContain('向右移动');
 expect(host.querySelector('[aria-live="assertive"]')?.textContent).toContain(node.dataset.id);
 expect(host.querySelector('[aria-live="assertive"]')?.textContent).toContain('x: 120');
 const position=node.style.transform,viewport=host.querySelector<HTMLElement>('.react-flow__viewport')!.style.transform,count=vi.mocked(fetch).mock.calls.length;
 const name=[...host.querySelectorAll('label.field')].find(label=>label.firstElementChild?.textContent==='工作流名称')!.querySelector('input')!;
 await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(name,'User draft 名称');name.dispatchEvent(new Event('input',{bubbles:true}));});
 node.focus();await act(async()=>{await setLocale('en-US',{persist:false});});
 expect(host.querySelector('.react-flow__node')).toBe(node);expect(document.activeElement).toBe(node);expect(node.classList.contains('selected')).toBe(true);
 expect(name.value).toBe('User draft 名称');expect(node.style.transform).toBe(position);expect(host.querySelector<HTMLElement>('.react-flow__viewport')!.style.transform).toBe(viewport);
 expect(node.getAttribute('aria-roledescription')).toBe('node');expect(description()).toContain('arrow keys');expect(vi.mocked(fetch).mock.calls).toHaveLength(count);
 await act(async()=>node.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true})));expect(node.classList.contains('selected')).toBe(false);
 await act(async()=>node.dispatchEvent(new KeyboardEvent('keydown',{key:' ',bubbles:true})));expect(node.classList.contains('selected')).toBe(true);
});

it('binds movement identity to the actual keyboard node across multiselect, locale and focus transitions',async()=>{
 await setLocale('en-US',{persist:false});
 await act(async()=>root.render(<WorkflowView project="p" refresh={0} onRefresh={()=>{}} onRun={()=>{}} onNode={()=>{}}/>));
 await act(async()=>[...host.querySelectorAll('button')].find(button=>button.textContent==='Add the first node')!.click());
 await act(async()=>[...host.querySelectorAll('button')].find(button=>button.textContent==='Add node')!.click());
 const nodes=[...host.querySelectorAll<HTMLElement>('.react-flow__node')];expect(nodes).toHaveLength(2);
 await act(async()=>nodes[0].click());
 await act(async()=>window.dispatchEvent(new KeyboardEvent('keydown',{key:'Control',code:'ControlLeft',ctrlKey:true,bubbles:true})));
 await act(async()=>nodes[1].dispatchEvent(new MouseEvent('click',{bubbles:true,ctrlKey:true})));
 await act(async()=>window.dispatchEvent(new KeyboardEvent('keyup',{key:'Control',code:'ControlLeft',bubbles:true})));
 expect(nodes.filter(node=>node.classList.contains('selected'))).toHaveLength(2);
 const inspector=[...host.querySelectorAll('select')].find(select=>nodes.every(node=>[...select.options].some(option=>option.value===node.dataset.id)))!;
 expect(inspector.value).toBe(nodes[1].dataset.id);
 const viewport=host.querySelector<HTMLElement>('.react-flow__viewport')!.style.transform;
 const count=vi.mocked(fetch).mock.calls.length;
 nodes[0].focus();await act(async()=>nodes[0].dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true})));
 expect(host.querySelector('[aria-live="assertive"]')?.textContent).toBe(`Moved node ${nodes[0].dataset.id} right. Starting coordinates: x: 120, y: 100`);
 expect(inspector.value).toBe(nodes[1].dataset.id);expect(nodes.filter(node=>node.classList.contains('selected'))).toHaveLength(2);
 const positions=nodes.map(node=>node.style.transform);
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(document.activeElement).toBe(nodes[0]);expect(nodes.map(node=>node.style.transform)).toEqual(positions);expect(inspector.value).toBe(nodes[1].dataset.id);
 // A focused outer node and the inspector still disagree after the locale rerender.
 await act(async()=>nodes[0].dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowDown',bubbles:true})));
 expect(host.querySelector('[aria-live="assertive"]')?.textContent).toBe(`节点 ${nodes[0].dataset.id} 向下移动。移动前坐标：x: 125，y: 100`);
 nodes[1].focus();await act(async()=>nodes[1].dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowLeft',bubbles:true})));
 expect(host.querySelector('[aria-live="assertive"]')?.textContent).toContain(`节点 ${nodes[1].dataset.id} 向左移动`);
 nodes[0].focus();await act(async()=>{await setLocale('en-US',{persist:false});});
 await act(async()=>nodes[0].dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowUp',bubbles:true})));
 expect(host.querySelector('[aria-live="assertive"]')?.textContent).toBe(`Moved node ${nodes[0].dataset.id} up. Starting coordinates: x: 120, y: 105`);
 expect(document.activeElement).toBe(nodes[0]);expect(nodes.filter(node=>node.classList.contains('selected'))).toHaveLength(2);
 expect(host.querySelector<HTMLElement>('.react-flow__viewport')!.style.transform).toBe(viewport);expect(vi.mocked(fetch).mock.calls).toHaveLength(count);
 // An app-owned attribute is not independent authority: an unknown ID must
 // never borrow the inspector's identity or the previous key event's ID.
 nodes[0].setAttribute('data-suma-keyboard-node','unknown-node');
 await act(async()=>nodes[0].dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true})));
 expect(host.querySelector('[aria-live="assertive"]')?.textContent).toBe('Moved node (identity unavailable) right. Starting coordinates: x: 120, y: 100');
 nodes[0].setAttribute('data-suma-keyboard-node',nodes[0].dataset.id!);
 await act(async()=>nodes[0].dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowLeft',bubbles:true})));
 expect(host.querySelector('[aria-live="assertive"]')?.textContent).toBe(`Moved node ${nodes[0].dataset.id} left. Starting coordinates: x: 125, y: 100`);
});

it('uses an explicit localized fallback when the public movement callback has no event-bound identity',async()=>{
 const input={direction:'right',x:120,y:100};
 expect(flowAriaLabelConfig(true)['node.a11yDescription.ariaLiveMessage'](input)).toBe('节点 （身份未确认） 向右移动。移动前坐标：x: 120，y: 100');
 await setLocale('en-US',{persist:false});
 expect(flowAriaLabelConfig(true,()=>undefined)['node.a11yDescription.ariaLiveMessage'](input)).toBe('Moved node (identity unavailable) right. Starting coordinates: x: 120, y: 100');
});

it('names actual input and output Handles with their raw node name and identity across locale changes',async()=>{
 await setLocale('en-US',{persist:false});
 await act(async()=>root.render(<WorkflowView project="p" refresh={0} onRefresh={()=>{}} onRun={()=>{}} onNode={()=>{}}/>));
 const click=async(text:string)=>{const button=[...host.querySelectorAll('button')].find(button=>button.textContent===text)!;expect(button).toBeTruthy();await act(async()=>button.click());};
 const rename=async(text:string)=>{
  const input=[...host.querySelectorAll('label.field')].find(label=>label.firstElementChild?.textContent==='Node name')!.querySelector('input')!;
  await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,text);input.dispatchEvent(new Event('input',{bubbles:true}));});return input;
 };
 const rawNames=['Raw alpha 用户 <script>','display.tool.profile_paused {{.reason}}'];
 await click('Add the first node');await rename(rawNames[0]);await click('Add node');const name=await rename(rawNames[1]);
 const nodes=[...host.querySelectorAll<HTMLElement>('.react-flow__node')];expect(nodes).toHaveLength(2);
 const handles=nodes.flatMap(node=>[...node.querySelectorAll<HTMLElement>('.react-flow__handle')]);expect(handles).toHaveLength(4);
 expect(handles.map(handle=>handle.getAttribute('aria-label'))).toEqual(nodes.flatMap((node,i)=>[
  `Input connection (target) for node ${rawNames[i]} (${node.dataset.id})`,
  `Output connection (source) for node ${rawNames[i]} (${node.dataset.id})`,
 ]));
 const positions=nodes.map(node=>node.style.transform),selection=nodes.map(node=>node.classList.contains('selected'));
 const viewport=host.querySelector<HTMLElement>('.react-flow__viewport')!.style.transform,count=vi.mocked(fetch).mock.calls.length;name.focus();
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(handles.map(handle=>handle.getAttribute('aria-label'))).toEqual(nodes.flatMap((node,i)=>[
  `节点 ${rawNames[i]}（${node.dataset.id}）的输入连接点（目标）`,
  `节点 ${rawNames[i]}（${node.dataset.id}）的输出连接点（来源）`,
 ]));
 expect(nodes.flatMap(node=>[...node.querySelectorAll('.react-flow__handle')])).toEqual(handles);
 expect(nodes.map(node=>node.style.transform)).toEqual(positions);expect(nodes.map(node=>node.classList.contains('selected'))).toEqual(selection);
 expect(host.querySelector<HTMLElement>('.react-flow__viewport')!.style.transform).toBe(viewport);
 expect(document.activeElement).toBe(name);expect(name.value).toBe(rawNames[1]);expect(vi.mocked(fetch).mock.calls).toHaveLength(count);expect(host.querySelector('script')).toBeNull();
});

it('keeps raw variable, nesting and replacement syntax literal in real Handles without resetting the dirty draft',async()=>{
 await setLocale('en-US',{persist:false});
 await act(async()=>root.render(<WorkflowView project="p" refresh={0} onRefresh={()=>{}} onRun={()=>{}} onNode={()=>{}}/>));
 await act(async()=>[...host.querySelectorAll('button')].find(button=>button.textContent==='Add the first node')!.click());
 const name=[...host.querySelectorAll('label.field')].find(label=>label.firstElementChild?.textContent==='Node name')!.querySelector('input')!;
 const node=host.querySelector<HTMLElement>('.react-flow__node')!,id=node.dataset.id!;
 const handles=[...node.querySelectorAll<HTMLElement>('.react-flow__handle')];
 const initial=handles.map(handle=>({role:handle.getAttribute('role'),tab:handle.getAttribute('tabindex'),cls:handle.className}));
 const position=node.style.transform,viewport=host.querySelector<HTMLElement>('.react-flow__viewport')!.style.transform,count=vi.mocked(fetch).mock.calls.length;
 const values=[
  '{{id}} $t(flow.zoom_in) " onmouseover="x <img src=x onerror=alert(1)> & 用户',
  '{{name}} {{id}} {{ name }} {{ id }} {{-name}} {{-id}}',
  '$t(flow.handle.input, {"name":"other","id":"other"})',
  '$& $$ $` $\' " <img> & 用户',
 ];
 for(const raw of values){
  await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(name,raw);name.dispatchEvent(new Event('input',{bubbles:true}));});name.focus();
  for(const locale of ['zh-CN','en-US']){
   await act(async()=>{await setLocale(locale,{persist:false});});
   expect(handles.map(handle=>handle.getAttribute('aria-label'))).toEqual(locale==='en-US'
    ? [`Input connection (target) for node ${raw} (${id})`,`Output connection (source) for node ${raw} (${id})`]
    : [`节点 ${raw}（${id}）的输入连接点（目标）`,`节点 ${raw}（${id}）的输出连接点（来源）`]);
   expect(name.value).toBe(raw);expect(document.activeElement).toBe(name);expect([...node.querySelectorAll('.react-flow__handle')]).toEqual(handles);
   expect(handles.map(handle=>({role:handle.getAttribute('role'),tab:handle.getAttribute('tabindex'),cls:handle.className}))).toEqual(initial);
   const event=new Event('suma:navigate',{cancelable:true});await act(async()=>window.dispatchEvent(event));expect(event.defaultPrevented).toBe(true);
  }
 }
 expect(host.querySelector('img,script,[onmouseover],[onerror]')).toBeNull();expect(vi.mocked(fetch).mock.calls).toHaveLength(count);
 expect(node.style.transform).toBe(position);expect(host.querySelector<HTMLElement>('.react-flow__viewport')!.style.transform).toBe(viewport);
});

it.each([
 {name:'{{id}}',id:'{{name}}'},
 {name:'{{name}}',id:'{{id}}'},
 {name:'{{ name }} {{ id }} {{-id}} {{name, uppercase}}',id:'{{id}} {{ name }} {{-name}}'},
 {name:'$t(flow.zoom_in)',id:'$t(flow.zoom_out)'},
 {name:'$& $$ $` $\'',id:'$$ $& $\' $`'},
 {name:'" onmouseover="x <img src=x onerror=alert(1)> & 用户',id:'" <script> & raw-id'},
])('inserts both raw Handle fields literally: %j',async({name,id})=>{
 for(const locale of ['en-US','zh-CN']){
  await setLocale(locale,{persist:false});
  expect(flowHandleLabel('input',name,id)).toBe(locale==='en-US'?`Input connection (target) for node ${name} (${id})`:`节点 ${name}（${id}）的输入连接点（目标）`);
  expect(flowHandleLabel('output',name,id)).toBe(locale==='en-US'?`Output connection (source) for node ${name} (${id})`:`节点 ${name}（${id}）的输出连接点（来源）`);
 }
});

it('describes official movement coordinates as the starting position while the actual node moves',async()=>{
 await setLocale('en-US',{persist:false});
 await act(async()=>root.render(<WorkflowView project="p" refresh={0} onRefresh={()=>{}} onRun={()=>{}} onNode={()=>{}}/>));
 await act(async()=>[...host.querySelectorAll('button')].find(button=>button.textContent==='Add the first node')!.click());
 const node=host.querySelector<HTMLElement>('.react-flow__node')!;node.focus();
 await act(async()=>node.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true})));
 expect(node.style.transform).toBe('translate(120px,100px)');
 await act(async()=>node.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowDown',bubbles:true})));
 expect(node.style.transform).toBe('translate(120px,105px)');
 expect(host.querySelector('[aria-live="assertive"]')?.textContent).toBe(`Moved node ${node.dataset.id} down. Starting coordinates: x: 120, y: 100`);
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 await act(async()=>node.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true})));
 expect(node.style.transform).toBe('translate(125px,105px)');
 expect(host.querySelector('[aria-live="assertive"]')?.textContent).toBe(`节点 ${node.dataset.id} 向右移动。移动前坐标：x: 120，y: 105`);
 expect(document.activeElement).toBe(node);expect(node.classList.contains('selected')).toBe(true);
});

function ReadOnlyGraph(){
 useLocale();
 const nodes:Node[]=[{id:'n1',position:{x:20,y:30},data:{label:'Original user node'},width:100,height:40,handles:[{id:'source',type:'source',position:Position.Right,x:100,y:20,width:1,height:1}]},
 {id:'n2',position:{x:220,y:30},data:{label:'Other user node'},width:100,height:40,handles:[{id:'target',type:'target',position:Position.Left,x:0,y:20,width:1,height:1}]}];
 return <ReactFlow defaultNodes={nodes} defaultEdges={[{id:'edge',source:'n1',target:'n2',sourceHandle:'source',targetHandle:'target',ariaLabel:'Pinned edge n1 to n2'}]} nodesDraggable={false} nodesConnectable={false} deleteKeyCode={null} ariaLabelConfig={flowAriaLabelConfig(false)}><Controls/><MiniMap/></ReactFlow>;
}
it('uses all real control labels and edge descriptions while readonly selection cannot move or delete nodes',async()=>{
 await act(async()=>root.render(<ReadOnlyGraph/>));
 const node=host.querySelector<HTMLElement>('[data-testid="rf__node-n1"]')!,edge=host.querySelector<HTMLElement>('[data-testid="rf__edge-edge"]')!;
 expect(node).not.toBeNull();expect(edge).not.toBeNull();
 const nodeDescription=document.getElementById(node.getAttribute('aria-describedby')!)!;
 const edgeDescription=document.getElementById(edge.getAttribute('aria-describedby')!)!;
 expect(nodeDescription.textContent).toBe('按 Enter 或空格键选择节点，按 Escape 取消选择。');
 expect(edgeDescription.textContent).toBe('按 Enter 或空格键选择连线，按 Escape 取消选择。');
 for(const [className,label] of [['zoomin','放大'],['zoomout','缩小'],['fitview','缩放以显示完整图'],['interactive','切换图交互']]){
  const button=host.querySelector<HTMLButtonElement>('.react-flow__controls-'+className)!;expect(button.title).toBe(label);expect(button.getAttribute('aria-label')).toBe(label);
 }
 expect(host.querySelector('.react-flow__minimap title')?.textContent).toBe('图缩略图');
 node.focus();await act(async()=>node.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true})));
 expect(node.classList.contains('selected')).toBe(true);const position=node.style.transform;
 await act(async()=>{node.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true}));node.dispatchEvent(new KeyboardEvent('keydown',{key:'Delete',bubbles:true}));});
 expect(node.style.transform).toBe(position);expect(host.querySelector('[data-testid="rf__node-n1"]')).toBe(node);expect(host.querySelector('[aria-live="assertive"]')?.textContent).toBe('');
 await act(async()=>{await setLocale('en-US',{persist:false});});
 expect(nodeDescription.textContent).toBe('Press Enter or Space to select a node and Escape to cancel selection.');expect(edgeDescription.textContent).toBe('Press Enter or Space to select an edge and Escape to cancel selection.');
 expect(host.querySelector<HTMLButtonElement>('.react-flow__controls-fitview')!.title).toBe('Fit graph to view');
 expect(node.style.transform).toBe(position);expect(document.activeElement).toBe(node);
});
