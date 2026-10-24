type Messages = Record<string,string>;
const pattern=/^\.\/domains\/([a-z][a-z0-9-]*)\.(zh-CN|en-US)\.json$/;

/** Domain owners add paired, flat-key catalogs. Assembly never allows an
 * import order to silently override another domain or the foundation. */
export function mergeLocaleCatalogs(base:Messages, modules:Record<string,unknown>, locale:'zh-CN'|'en-US'):Messages {
  const result:Messages=Object.assign(Object.create(null),base);
  for(const [file,value] of Object.entries(modules).sort(([a],[b])=>a.localeCompare(b))){
    const parsed=file.match(pattern);
    if(!parsed)throw Error('Invalid domain catalog path: '+file);
    const [,domain,language]=parsed,other=language==='zh-CN'?'en-US':'zh-CN';
    const pair=modules['./domains/'+domain+'.'+other+'.json'];
    if(!pair)throw Error('Missing domain catalog pair: '+file);
    if(!value||Array.isArray(value)||typeof value!=='object'||!pair||Array.isArray(pair)||typeof pair!=='object')throw Error('Domain catalog must contain string messages: '+file);
    const messages=value as Record<string,unknown>,paired=pair as Record<string,unknown>;
    if(JSON.stringify(Object.keys(messages).sort())!==JSON.stringify(Object.keys(paired).sort()))throw Error('Domain catalog key mismatch: '+domain);
    for(const [key,text] of Object.entries(messages)){
      if(!key.startsWith(domain+'.'))throw Error('Domain catalog namespace mismatch: '+key);
      if(typeof text!=='string'||!text.trim()||text.includes('\uFFFD'))throw Error('Domain catalog requires nonempty string messages: '+key);
      if(language===locale){
        if(Object.hasOwn(result,key))throw Error('duplicate catalog key: '+key);
        result[key]=text;
      }
    }
  }
  return result;
}
