import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {afterEach,describe,expect,it,vi} from 'vitest';
import {api} from './api';
import {parseWorkbenchRoute,initializeSessionRoute} from './navigation';
import {CapacityView,CapacityWeekPicker} from './Planning';
import {applyCapacityWeekInput,capacityScopeMatches,isCapacityWeek,planningRequestPath,type Capacity} from './planning-model';
import {modalFocusTarget} from './modal-focus';
afterEach(()=>vi.unstubAllGlobals());
const row:Capacity={member_id:'resource-a',name:'真实资源',kind:'human',week_start:'2026-10-19',version:1,available_hours:40,allocated_hours:8,unestimated_tasks:0};
describe('capacity week native input and route consumers',()=>{
 it('uses the native widget value to update the shared route and actual API request week',async()=>{
  let route=parseWorkbenchRoute('?project=project-a&view=development&sub=planning&layout=capacity&week=2026-10-19');
  const widget={value:'2026-10-26'};
  expect(applyCapacityWeekInput(widget,route.week,week=>{route={...route,week};return true;})).toBe('');
  const query=new URLSearchParams(Object.entries(route)).toString();
  const restored=initializeSessionRoute(parseWorkbenchRoute('?'+query),[{id:'project-a'}]);
  expect(restored).toMatchObject({project:'project-a',sub:'planning',layout:'capacity',week:'2026-10-26'});
  const fetcher=vi.fn().mockResolvedValue(new Response(JSON.stringify({capacity:[{...row,week_start:'2026-10-26',available_hours:null,allocated_hours:0}]}),{status:200}));vi.stubGlobal('fetch',fetcher);
  const result=await api<{capacity:Capacity[]}>(planningRequestPath(restored.project,restored.week));
  expect(fetcher.mock.calls[0][0]).toBe('/api/v1/planning?project_id=project-a&week_start=2026-10-26');
  expect(capacityScopeMatches('2026-10-26',restored.week,result.capacity)).toBe(true);
 });
 it('restores the last valid widget and leaves the route unchanged for non-Mondays, nonexistent dates and blank input',()=>{
  for(const value of ['2026-10-27','2026-02-30','','not-a-date']){
   const onWeek=vi.fn(),widget={value};const error=applyCapacityWeekInput(widget,'2026-10-19',onWeek);
   expect(error).toContain('有效的周一');expect(error).toContain('2026-10-19');expect(widget.value).toBe('2026-10-19');expect(onWeek).not.toHaveBeenCalled();
   expect(applyCapacityWeekInput(widget,'2026-10-19',onWeek)).toBeUndefined();expect(onWeek).not.toHaveBeenCalled();
  }
  expect(isCapacityWeek('2026-02-30')).toBe(false);expect(isCapacityWeek('2026-10-26')).toBe(true);
 });
 it('restores the widget when an unfinished write blocks navigation',()=>{
  const widget={value:'2026-10-26'};expect(applyCapacityWeekInput(widget,'2026-10-19',()=>false)).toContain('尚未完成');expect(widget.value).toBe('2026-10-19');
 });
 it('never renders old-week rows under a new week while loading or after a failed read',()=>{
  const ready=capacityScopeMatches('2026-10-19','2026-10-26',[row]);expect(ready).toBe(false);
  for(const pending of [true,false]){
   const html=renderToStaticMarkup(createElement(CapacityView,{project:'project-a',week:'2026-10-26',onWeek:()=>true,rows:[row],ready,pending,routeError:'',onRefresh:()=>{}}));
   expect(html).not.toContain('40 h');expect(html).not.toContain('8 h');expect(html).not.toContain('设置容量</button>');expect(html).toContain(pending?'正在读取':'本周容量尚未读取');
  }
  expect(capacityScopeMatches('2026-10-26','2026-10-26',[row])).toBe(false);
  expect(capacityScopeMatches('2026-10-26','2026-10-26',[])).toBe(true);
 });
 it('renders a named week control and preserves an invalid URL value for an explicit validation error',()=>{
  expect(parseWorkbenchRoute('?view=development&sub=planning&layout=capacity&week=2026-10-26').week).toBe('2026-10-26');
  expect(parseWorkbenchRoute('?week=2026-10-27').week).toBe('2026-10-27');
  const html=renderToStaticMarkup(createElement(CapacityWeekPicker,{week:'2026-10-26',onWeek:()=>true,onError:()=>{}}));
  expect(html).toContain('name="week"');expect(html).toContain('value="2026-10-26"');
  expect(()=>planningRequestPath('project-a','2026-10-27')).toThrow('有效的周一');
 });
});
describe('modal focus return policy',()=>{
 it('returns to the connected opener and uses main when that opener was removed or became unavailable',()=>{
  const main={target:'main',connected:true,available:true};
  expect(modalFocusTarget({target:'task-button',connected:true,available:true},main)).toBe('task-button');
  expect(modalFocusTarget({target:'deleted-button',connected:false,available:true},main)).toBe('main');
  expect(modalFocusTarget({target:'disabled-or-hidden-button',connected:true,available:false},main)).toBe('main');
  expect(modalFocusTarget(undefined,{target:'unmounted-main',connected:false,available:true})).toBeUndefined();
 });
});
