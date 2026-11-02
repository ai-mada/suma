import type {InstalledToolFact} from './tool-quick-connect-model';

/** Presentation only: callers supply facts already accepted by the existing
 * scoped scan reader. This neither validates raw input nor changes authority,
 * installation evidence, launch support, or the original scan's lifetime. */
export function projectLocalToolsScan(items:readonly InstalledToolFact[],selectedAdapterID?:string):{primary:InstalledToolFact[];other:InstalledToolFact[];detectedCount:number;attentionCount:number}{
 const primary:InstalledToolFact[]=[],other:InstalledToolFact[]=[];
 let detectedCount=0,attentionCount=0;
 for(const item of items){
  // Only an explicit ordinary absence may be folded. Keep the exact original
  // object and relative order; equal products can describe distinct protocols.
  const ordinaryAbsence=item.installed===false&&item.reason_code==='not_installed';
  if(ordinaryAbsence&&item.adapter_id!==selectedAdapterID)other.push(item);
  else primary.push(item);
  if(item.installed===true)detectedCount++;
  else if(item.installed===false&&!ordinaryAbsence)attentionCount++;
 }
 return {primary,other,detectedCount,attentionCount};
}
