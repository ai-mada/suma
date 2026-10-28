import {createContext,useCallback,useContext,type ReactNode} from 'react';
import {api,ApiError,type ApiCaller,type Session} from './api';
import {UiError} from './i18n';
import {mutate} from './operations';

type View=Readonly<{userID:string;organizationID:string}>;
type ViewSession=Pick<Session,'user'|'organization'>;
const ApiContext=createContext<ApiCaller|undefined>(undefined);
const ViewContext=createContext<View|undefined>(undefined);
const userHeader='X-Suma-Expected-User',organizationHeader='X-Suma-Expected-Organization';
const bootstrap=new Set(['/auth/setup','/auth/login','/auth/invitations/accept','/auth/logout']);

function conflict():never{
 const diagnostic=new UiError('common.viewContextConflict');
 throw new ApiError(403,'view_context_conflict',diagnostic.message,undefined,undefined,diagnostic);
}
async function send<T>(transport:ApiCaller,view:View|undefined,args:Parameters<ApiCaller>):Promise<T>{
 const [path,body,method,headers]=args,route=path.split('?')[0];
 if(['GET','HEAD','OPTIONS'].includes((method??(body?'POST':'GET')).toUpperCase())||bootstrap.has(route))return transport<T>(...args);
 // No loaded view means no invented identity. The server's required-view guard
 // rejects this request with 428; in particular, body/cookie state is not a source.
 const explicit=new Map<string,string>(),merged:Record<string,string>={};
 for(const [name,value] of Object.entries(headers??{})){
  const key=name.toLowerCase();
  if(key===userHeader.toLowerCase()||key===organizationHeader.toLowerCase()){
   if(explicit.has(key)&&explicit.get(key)!==value)conflict();
   explicit.set(key,value);
  }else merged[name]=value;
 }
 const user=explicit.get(userHeader.toLowerCase()),organization=explicit.get(organizationHeader.toLowerCase());
 if(route==='/account/preferences'){
  // Account preferences are user-wide. Preserve the writer's original user,
  // including when an organization Provider changed while its receipt was lost.
  if(organization!==undefined)conflict();
  if(user!==undefined||view)merged[userHeader]=user??view!.userID;
 }else if(user!==undefined&&organization!==undefined){
  // An explicit complete original scope wins, never a later Provider value.
  merged[userHeader]=user;merged[organizationHeader]=organization;
 }else{
  if(view&&((user!==undefined&&user!==view.userID)||(organization!==undefined&&organization!==view.organizationID)))conflict();
  if(user!==undefined||view)merged[userHeader]=user??view!.userID;
  if(organization!==undefined||view)merged[organizationHeader]=organization??view!.organizationID;
 }
 const request=[...args] as Parameters<ApiCaller>;request[3]=merged;
 return transport<T>(...request);
}

/** Captures primitives now, not a mutable session object or a future cookie. */
export function bindApiView(session:ViewSession|null|undefined,transport:ApiCaller=api):ApiCaller{
 const userID=session?.user.id,organizationID=session?.organization.id;
 const view=userID&&organizationID?Object.freeze({userID,organizationID}):undefined;
 return <T=any>(...args:Parameters<ApiCaller>)=>send<T>(transport,view,args);
}
export function useViewApi(session:ViewSession|null|undefined):ApiCaller{
 const userID=session?.user.id,organizationID=session?.organization.id;
 return useCallback(<T=any>(...args:Parameters<ApiCaller>)=>send<T>(api,userID&&organizationID?{userID,organizationID}:undefined,args),[userID,organizationID]);
}
export function ApiViewProvider({session,children}:{session:ViewSession|null;children:ReactNode}){
 const caller=useViewApi(session);
 const userID=session?.user.id,organizationID=session?.organization.id;
 const view=userID&&organizationID?Object.freeze({userID,organizationID}):undefined;
 return <ViewContext.Provider value={view}><ApiContext.Provider value={caller}>{children}</ApiContext.Provider></ViewContext.Provider>;
}
/** The raw fallback carries no view; it cannot guess a user outside a Provider. */
export function useApi():ApiCaller{return useContext(ApiContext)??api;}
/** Native selection binds these original primitives; it never receives a
 * session cookie, authentication token or permission inferred from the UI. */
export function useApiView():View|undefined{return useContext(ViewContext);}
export function useMutationApi(){
 const caller=useApi();
 return useCallback(<T=any>(path:string,body:Record<string,unknown>,method?:string,identity?:string)=>mutate<T>(path,body,method,identity,caller),[caller]);
}
