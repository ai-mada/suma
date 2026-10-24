import type {RecordItem} from './api';
export function withCurrentOption(options:RecordItem[],id:string|undefined,kind:string):RecordItem[]{
 if(!id||options.some(option=>option.id===id))return options;
 return [{id,version:0,name:`当前${kind}（未在已加载范围）`,title:`当前${kind}（未在已加载范围）`},...options];
}
