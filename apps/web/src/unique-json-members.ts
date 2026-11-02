/** Requires syntactically valid JSON (call JSON.parse first). This lexical
 * pass preserves duplicate member detection before parsed projections are used. */
export function uniqueJSONMembers(source:string):boolean{
 const stack:({kind:'object';keys:Set<string>;key:boolean}|{kind:'array'})[]=[];
 for(let i=0;i<source.length;i++){
  const c=source[i];if(c==='"'){const start=i++;while(i<source.length){if(source[i]==='\\'){i+=2;continue;}if(source[i]==='"')break;i++;}const top=stack.at(-1);if(top?.kind==='object'&&top.key){const key=JSON.parse(source.slice(start,i+1)) as string;if(top.keys.has(key))return false;top.keys.add(key);top.key=false;}continue;}
  if(c==='{')stack.push({kind:'object',keys:new Set(),key:true});else if(c==='[')stack.push({kind:'array'});else if(c==='}'||c===']')stack.pop();else if(c===','&&stack.at(-1)?.kind==='object')(stack.at(-1) as {kind:'object';keys:Set<string>;key:boolean}).key=true;
 }
 return true;
}
