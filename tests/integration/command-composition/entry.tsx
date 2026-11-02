import {createRoot} from 'react-dom/client';
import '../../../apps/web/src/style.css';
import '../../../apps/web/src/experience-foundation.css';
import {CommandCompositionSetup} from '../../../apps/web/src/CommandCompositionSetup';
import {LanguageProvider} from '../../../apps/web/src/i18n/LanguageSelector';
import {setLocale} from '../../../apps/web/src/i18n';

const parameters=new URLSearchParams(location.search),locale=parameters.get('locale')==='en-US'?'en-US':'zh-CN';
document.documentElement.lang=locale;
document.documentElement.dataset.theme=parameters.get('theme')==='dark'?'dark':'light';
void setLocale(locale,{persist:false}).then(()=>{
 createRoot(document.getElementById('root')!).render(<LanguageProvider><main style={{maxWidth:960,margin:'24px auto',padding:16}}>
  <header><strong>suma 1.0.0</strong><h1>{locale==='zh-CN'?'组合已有工具配置':'Combine existing tool configurations'}</h1></header>
  <CommandCompositionSetup/>
 </main></LanguageProvider>);
});
