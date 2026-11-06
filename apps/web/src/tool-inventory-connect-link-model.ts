import {readAdapterCatalog,type AdapterDescriptor} from './AdapterConnections';
import {UiError} from './i18n';
import {toolConnectObservationMatches,validToolConnectObservationPin,type ToolConnectObservationPin} from './tool-connect-observation-model';
import {parseToolDiscovery} from './tool-discovery-model';
import {matchingQuickTools,quickConnectAdapters} from './tool-quick-connect-model';
import type {ToolConnectDraft} from './ToolQuickConnect';

export const toolInventoryConnectPrefix='tool-inventory-connect:';
const locator=/^([A-Za-z0-9][A-Za-z0-9_.-]{0,99}):([A-Za-z0-9][A-Za-z0-9_.-]{0,99}):([A-Za-z0-9][A-Za-z0-9_.-]{0,99}):([a-f0-9]{64}):([^\s]+)$/;

type InventoryConnectPin=Readonly<{
 projectID:string;
 workerID:string;
 commandID:string;
 observation:ToolConnectObservationPin;
}>;
export type ToolInventoryConnectReview={adapter:AdapterDescriptor;draft:ToolConnectDraft};

/** The locator carries only a selection. Current browser permissions and
 * freshly read server evidence determine what can enter Quick Connect. */
export function parseToolInventoryConnectLink(panel:string,project:string):InventoryConnectPin|undefined {
 if(!panel.startsWith(toolInventoryConnectPrefix)||panel.length>430)return;
 const suffix=panel.slice(toolInventoryConnectPrefix.length);
 const parsed=locator.exec(suffix);
 if(!parsed||parsed[0].length!==suffix.length||parsed[1]!==project)return;
 const [,projectID,workerID,commandID,profileSHA256,observedAt]=parsed;
 const observation=Object.freeze({profileSHA256,observedAt});
 if(!validToolConnectObservationPin(observation))return;
 return Object.freeze({projectID,workerID,commandID,observation});
}

export function reviewToolInventoryConnectLink(pin:InventoryConnectPin,report:unknown,catalog:unknown,elapsedMs:number):ToolInventoryConnectReview {
 const view=parseToolDiscovery(report,pin.projectID,pin.workerID);
 if(view.state!=='fresh'||!view.observedAt||view.remainingMs<=elapsedMs){
  throw new UiError('tool-inventory-connect-link.stale');
 }
 const observation=view.items.find(item=>item.profile.command_id===pin.commandID);
 if(!observation||!toolConnectObservationMatches(pin.observation,view,observation)){
  throw new UiError('tool-inventory-connect-link.changed');
 }
 const page=readAdapterCatalog(catalog,pin.projectID);
 const matches=quickConnectAdapters(page?.items??[]).filter(adapter=>matchingQuickTools([observation],adapter).length===1);
 if(matches.length!==1)throw new UiError('tool-inventory-connect-link.adapter_unavailable');
 const [adapter]=matches;
 return {adapter,draft:{adapterID:adapter.id,workerID:pin.workerID,commandID:pin.commandID,name:'',nameEdited:false,observationPin:Object.freeze({
  profileSHA256:pin.observation.profileSHA256,observedAt:view.observedAt,
 })}};
}
