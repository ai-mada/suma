import {managementAreas} from './management-navigation';
import {sourceDestination,type WorkbenchView} from './navigation';

export type CommandPaletteTarget={view:WorkbenchView;sub:string;panel?:string;layout?:string};
export type PalettePage={id:string;labelKey:string;keywordsKey:string;target:CommandPaletteTarget;projectRequired:boolean};
export type PaletteSearchRow={id:string;title:string;snippet:string;kind:string;revision?:number;source:unknown};
export type PaletteSearchResult={items:PaletteSearchRow[];total:number;limit:number;truncated:boolean};

const projectPages:PalettePage[]=[
 {id:'overview',labelKey:'command-palette.page.overview',keywordsKey:'command-palette.keywords.overview',target:{view:'overview',sub:''},projectRequired:true},
 {id:'my-work',labelKey:'command-palette.page.my-work',keywordsKey:'command-palette.keywords.my-work',target:{view:'development',sub:'my-work'},projectRequired:true},
 {id:'tasks',labelKey:'shell.tabs.tasks',keywordsKey:'command-palette.keywords.tasks',target:{view:'development',sub:'tasks'},projectRequired:true},
 {id:'ide',labelKey:'shell.tabs.workspace',keywordsKey:'command-palette.keywords.ide',target:{view:'development',sub:'ide'},projectRequired:true},
 {id:'runs',labelKey:'shell.tabs.runs',keywordsKey:'command-palette.keywords.runs',target:{view:'development',sub:'runs'},projectRequired:true},
 {id:'planning',labelKey:'shell.tabs.planning',keywordsKey:'command-palette.keywords.planning',target:{view:'development',sub:'planning'},projectRequired:true},
 {id:'workflows',labelKey:'shell.tabs.workflows',keywordsKey:'command-palette.keywords.workflows',target:{view:'development',sub:'workflows'},projectRequired:true},
 {id:'requirements',labelKey:'command-palette.page.requirements',keywordsKey:'command-palette.keywords.requirements',target:{view:'requirements',sub:''},projectRequired:true},
 {id:'issues',labelKey:'shell.tabs.issue_intake',keywordsKey:'command-palette.keywords.issues',target:{view:'quality',sub:'issues'},projectRequired:true},
 {id:'documents',labelKey:'shell.tabs.documents',keywordsKey:'command-palette.keywords.documents',target:{view:'knowledge',sub:'documents'},projectRequired:true},
];

/** Management entries follow the same catalog as the sidebar. Navigation
 * reveals no resource data; each destination still checks current authority. */
export const palettePages:readonly PalettePage[]=[...projectPages,...managementAreas.map(area=>({
 id:'management-'+area.id,labelKey:'management.area.'+area.id,
 keywordsKey:'command-palette.keywords.management-'+area.id,
 target:{view:'settings' as const,sub:area.start},projectRequired:false,
}))];

export function pageMatches(label:string,keywords:string,query:string):boolean{
 const words=query.trim().toLocaleLowerCase().split(/\s+/u).filter(Boolean);
 const text=(label+' '+keywords).toLocaleLowerCase();
 return words.every(word=>text.includes(word));
}

export function paletteQueryValid(query:string):boolean{
 return Array.from(query).length<=200&&new TextEncoder().encode(query).length<=500;
}

/** Search locators are data from the project authority, never executable URLs.
 * Unknown and foreign kinds cannot become an actionable result. */
export function paletteSearchResult(project:string,value:unknown):PaletteSearchResult{
 if(!value||typeof value!=='object'||Array.isArray(value))throw new Error('Invalid search response');
 const response=value as Record<string,unknown>;
 if(!Array.isArray(response.items))throw new Error('Invalid search response');
 const items:PaletteSearchRow[]=[],seen=new Set<string>();
 for(const source of response.items.slice(0,50)){
  const destination=sourceDestination(project,source);
  if(!destination||!source||typeof source!=='object'||Array.isArray(source))continue;
  const row=source as Record<string,unknown>;
  if(typeof row.title!=='string'||!row.title.trim())continue;
  const id=destination.sub+':'+destination.record;
  if(seen.has(id))continue;
  seen.add(id);items.push({id,title:row.title.slice(0,200),snippet:typeof row.snippet==='string'?row.snippet.slice(0,240):'',kind:destination.sub||'requirements',revision:Number.isSafeInteger(row.revision)&&Number(row.revision)>0?Number(row.revision):undefined,source});
 }
 const total=Number.isSafeInteger(response.total)&&Number(response.total)>=0?Number(response.total):items.length;
 const limit=Number.isSafeInteger(response.limit)&&Number(response.limit)>0?Math.min(50,Number(response.limit)):50;
 return {items,total,limit,truncated:response.truncated===true||total>response.items.length};
}
