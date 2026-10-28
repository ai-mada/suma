import {createHash} from 'node:crypto';

export interface ProviderRequest {body:string;requestSHA256:string}
export interface ProviderResponse {data:unknown;responseSHA256:string}
export interface ProviderAdapter {prepare(body:Record<string,unknown>):ProviderRequest;invoke(request:ProviderRequest,signal:AbortSignal,step?:number):Promise<ProviderResponse>}
type BoundedTransport=(url:string,init:RequestInit,signal:AbortSignal,exactUsage:boolean)=>Promise<unknown>;

// OpenAI-compatible protocol adapter over the existing bounded, cancellable,
// no-retry transport. It is not an official vendor SDK or billing integration.
export class OpenAICompatibleProviderAdapter implements ProviderAdapter {
 constructor(private readonly model:string,private readonly baseURL:string,private readonly apiKey:string|undefined,private readonly transport:BoundedTransport){}
 prepare(input:Record<string,unknown>):ProviderRequest {const body=JSON.stringify({...input,model:this.model});return {body,requestSHA256:createHash('sha256').update(body,'utf8').digest('hex')};}
 async invoke(request:ProviderRequest,signal:AbortSignal,step?:number):Promise<ProviderResponse>{return await this.transport(`${this.baseURL}/chat/completions`,{method:'POST',headers:{'Content-Type':'application/json',...(this.apiKey?{Authorization:`Bearer ${this.apiKey}`} :{}),...(step!==undefined?{'X-Suma-Model-Step':String(step)}:{})},body:request.body},signal,true) as ProviderResponse;}
}
