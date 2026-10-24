import {Component,type ReactNode} from 'react';
import {AlertTriangle,RefreshCw,ArrowLeft} from 'lucide-react';
import {tr as t,useLocale} from './i18n';

export function recoveryDetails(error:Error,url:string){
 const resourceError=/dynamically imported module|importing a module script|loading chunk|loading css chunk|module script.*failed/i.test(error.message);
 const home=new URL(url);home.searchParams.set('view','overview');for(const key of ['sub','run','node','record'])home.searchParams.delete(key);
 return {title:t(resourceError?'workspace.recovery.resource_title':'workspace.recovery.error_title'),description:t(resourceError?'workspace.recovery.resource_description':'workspace.recovery.error_description'),currentURL:url,homeURL:home.toString()};
}

// The mounted fallback subscribes to the locale. The class keeps the original
// failure and never resets or retries a broken subtree on language changes.
export function ErrorRecovery({error,inline=false}:{error:Error;inline?:boolean}){
 useLocale();
 const details=recoveryDetails(error,location.href);
 return <section className={'error-recovery '+(inline?'inline-recovery':'')} role="alert"><span className="recovery-icon"><AlertTriangle size={25}/></span><span className="eyebrow">{t('workspace.recovery.eyebrow')}</span><h1>{details.title}</h1><p>{details.description}</p><p className="muted">{t('workspace.recovery.preserved')}</p><code className="recovery-url">{details.currentURL}</code><div className="recovery-actions"><button className="primary" onClick={()=>location.reload()}><RefreshCw size={15}/>{t('workspace.recovery.reload')}</button><a className="recovery-link" href={details.homeURL}><ArrowLeft size={15}/>{t('workspace.recovery.home')}</a></div><details><summary>{t('workspace.recovery.details')}</summary><pre>{error.stack||error.message}</pre></details></section>;
}

export class ErrorBoundary extends Component<{children:ReactNode;inline?:boolean},{error:Error|null}>{
 state:{error:Error|null}={error:null};
 static getDerivedStateFromError(error:Error){return {error};}
 render(){return this.state.error?<ErrorRecovery error={this.state.error} inline={this.props.inline}/>:this.props.children;}
}
