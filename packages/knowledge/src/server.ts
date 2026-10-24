import {createServer,type IncomingMessage,type Server} from 'node:http';
import {timingSafeEqual} from 'node:crypto';
import {resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {KnowledgeIndex} from './index.js';
import type {IndexOptions,IndexRequest,SearchRequest} from './types.js';
export function createKnowledgeServer(index:KnowledgeIndex,token:string):Server{
 if(!token.trim())throw Error('SUMA_KNOWLEDGE_TOKEN is required');const expected=Buffer.from(`Bearer ${token}`);
 return createServer(async(req,res)=>{res.setHeader('Content-Type','application/json; charset=utf-8');res.setHeader('Cache-Control','no-store');res.setHeader('X-Content-Type-Options','nosniff');const send=(status:number,value:unknown)=>{res.writeHead(status);res.end(JSON.stringify(value))};
 try{const url=new URL(req.url??'/',`http://localhost`);if(req.method==='GET' && url.pathname==='/health'){try{send(200,{status:'ok',...await index.health()})}catch{send(503,{status:'unavailable',available:false,verified:false})}return;}
 const actual=Buffer.from(req.headers.authorization??'');if(actual.length!==expected.length || !timingSafeEqual(actual,expected)){send(401,{error:{code:'unauthorized',message:'Internal service token required'}});return;}
 if(req.method!=='POST' || !['/index','/search'].includes(url.pathname)){send(404,{error:{code:'not_found',message:'Route not found'}});return;}
 if(!req.headers['content-type']?.startsWith('application/json')){send(415,{error:{code:'content_type',message:'JSON required'}});return;}
 const body=await readJSON(req);if(url.pathname==='/index')send(200,await index.index(body as IndexRequest));else send(200,await index.search(body as SearchRequest));
 }catch(e){const message=e instanceof Error?e.message:'';const validation=/Invalid|invalid|Positive|Title\/content|exceeds/.test(message);send(validation?400:503,{error:{code:validation?'invalid_request':'index_unavailable',message:validation?message:'Native index operation failed; check service configuration and retry'}});}
 });
}
async function readJSON(req:IncomingMessage):Promise<unknown>{let bytes=0;const chunks:Buffer[]=[];for await(const chunk of req){bytes+=chunk.length;if(bytes>2*1024*1024)throw Error('Request exceeds 2 MiB');chunks.push(Buffer.from(chunk))}try{const body:unknown=JSON.parse(Buffer.concat(chunks).toString('utf8'));if(!body || typeof body!=='object' || Array.isArray(body))throw Error();return body}catch{throw Error('Invalid JSON object')}}
async function main(){const token=process.env.SUMA_KNOWLEDGE_TOKEN;if(!token)throw Error('SUMA_KNOWLEDGE_TOKEN is required');const options:IndexOptions={directory:resolve(process.env.SUMA_KNOWLEDGE_DIR??'.suma/knowledge'),backend:process.env.SUMA_KNOWLEDGE_BACKEND==='oceanbase'?'oceanbase':'lancedb',oceanbaseURL:process.env.SUMA_KNOWLEDGE_OB_URL,oceanbaseTLS:process.env.SUMA_KNOWLEDGE_OB_TLS==='true',...(process.env.SUMA_EMBEDDING_URL?{embedding:{url:process.env.SUMA_EMBEDDING_URL,model:process.env.SUMA_EMBEDDING_MODEL??'',dimensions:Number(process.env.SUMA_EMBEDDING_DIMENSIONS),version:process.env.SUMA_EMBEDDING_VERSION??'',apiKey:process.env.SUMA_EMBEDDING_API_KEY}}:{})};const index=await KnowledgeIndex.open(options);const server=createKnowledgeServer(index,token);const port=Number(process.env.SUMA_KNOWLEDGE_PORT??4312);if(!Number.isInteger(port) || port<1 || port>65535)throw Error('Invalid knowledge port');server.listen(port,process.env.SUMA_KNOWLEDGE_HOST??'127.0.0.1');const shutdown=()=>{server.close(()=>{void index.close().then(()=>{process.exitCode=0}).catch(()=>{process.exitCode=1})})};process.once('SIGINT',shutdown);process.once('SIGTERM',shutdown);server.on('error',()=>{void index.close();process.exitCode=1});}
if(process.argv[1] && resolve(process.argv[1])===fileURLToPath(import.meta.url))void main().catch(()=>{process.stderr.write('Knowledge service startup failed; check native backend and required environment configuration.\n');process.exitCode=1});
