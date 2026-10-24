import {Component,type ReactNode} from 'react';
import {AlertTriangle,RefreshCw,ArrowLeft} from 'lucide-react';

export function recoveryDetails(error:Error,url:string){
 const resourceError=/dynamically imported module|importing a module script|loading chunk|loading css chunk|module script.*failed/i.test(error.message);
 const home=new URL(url);home.searchParams.set('view','overview');for(const key of ['sub','run','node','record'])home.searchParams.delete(key);
 return {title:resourceError?'页面资源已更新':'页面暂时无法显示',description:resourceError?'当前标签页引用的旧版本资源已不可用。请手动加载当前版本，继续原来的项目与页面。':'界面遇到了错误。你可以重新加载当前页面，或返回项目概览。',currentURL:url,homeURL:home.toString()};
}

export function ErrorRecovery({error,inline=false}:{error:Error;inline?:boolean}){
 const details=recoveryDetails(error,location.href);
 return <section className={'error-recovery '+(inline?'inline-recovery':'')} role="alert"><span className="recovery-icon"><AlertTriangle size={25}/></span><span className="eyebrow">WORKSPACE RECOVERY</span><h1>{details.title}</h1><p>{details.description}</p><p className="muted">当前地址已保留。已保存到项目的内容会从服务端重新读取；页面不会自动刷新。</p><code className="recovery-url">{details.currentURL}</code><div className="recovery-actions"><button className="primary" onClick={()=>location.reload()}><RefreshCw size={15}/>重新加载当前页面</button><a className="recovery-link" href={details.homeURL}><ArrowLeft size={15}/>返回项目概览</a></div><details><summary>错误详情</summary><pre>{error.message}</pre></details></section>
}

export class ErrorBoundary extends Component<{children:ReactNode;inline?:boolean},{error:Error|null}>{
 state:{error:Error|null}={error:null};
 static getDerivedStateFromError(error:Error){return {error};}
 render(){return this.state.error?<ErrorRecovery error={this.state.error} inline={this.props.inline}/>:this.props.children;}
}
