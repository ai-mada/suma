import {ChevronLeft,ChevronRight,ChevronsLeft,RefreshCw} from 'lucide-react';
import {formatNumber,tr} from './i18n';
import './DispatchPageNavigation.css';

export const firstDispatchPage: readonly string[]=[''];

type Props={
 label:string;
 number:number;
 loading:boolean;
 failed:boolean;
 summary?:string;
 onFirst?:()=>void;
 onPrevious?:()=>void;
 onNext?:()=>void;
 onRefresh?:()=>void;
};

export function DispatchPageNavigation({label,number,loading,failed,summary,onFirst,onPrevious,onNext,onRefresh}:Props){
 const first=tr('dispatch-pages.first'),previous=tr('dispatch-pages.previous'),next=tr('dispatch-pages.next');
 const reload=tr(failed?'dispatch-pages.retry':'dispatch-pages.refresh');
 const status=tr(loading?'dispatch-pages.loading':failed?'dispatch-pages.unconfirmed':'dispatch-pages.current',{page:formatNumber(number)});
 // The current control stays focused when a page starts or stops loading.
 // Unavailable controls leave the tab sequence and have no click handler.
 return <div className="dispatch-page-navigation" role="group" aria-label={label}>
  <div className="dispatch-page-controls">
   <button type="button" data-dispatch-page="first" aria-label={first} title={first} aria-disabled={!onFirst} tabIndex={onFirst?0:-1} onClick={onFirst}><ChevronsLeft size={16} aria-hidden="true"/></button>
   <button type="button" data-dispatch-page="previous" aria-label={previous} title={previous} aria-disabled={!onPrevious} tabIndex={onPrevious?0:-1} onClick={onPrevious}><ChevronLeft size={16} aria-hidden="true"/></button>
   <span className="dispatch-page-status" role="status" aria-atomic="true">{status}{summary&&<small>{summary}</small>}</span>
   <button type="button" data-dispatch-page="next" aria-label={next} title={next} aria-disabled={!onNext} tabIndex={onNext?0:-1} onClick={onNext}><ChevronRight size={16} aria-hidden="true"/></button>
  </div>
  <button type="button" data-dispatch-page="refresh" aria-disabled={!onRefresh} tabIndex={onRefresh?0:-1} onClick={onRefresh}><RefreshCw size={14} aria-hidden="true"/>{reload}</button>
 </div>;
}
