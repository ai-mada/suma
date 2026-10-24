import {tr} from "./i18n";
import type {RecordItem} from './api';
export function withCurrentOption(options:RecordItem[],id:string|undefined,kind:string):RecordItem[]{
 if(!id||options.some(option=>option.id===id))return options;
 return [{id,version:0,name:tr("records.current_reference",{kind}),title:tr("records.current_reference",{kind})},...options];
}
