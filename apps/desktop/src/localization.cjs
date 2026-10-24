'use strict';
// Electron 的本机语言仅是显示偏好，不能改变控制面身份、业务内容或执行日志。
// Intl.Locale 是 Node/Electron 内置的 BCP 47 实现，不需要额外运行时依赖。
function normalizeLocale(value){
 if(typeof value!=='string'||value.length>128||/[\u0000-\u001f\u007f]/.test(value))return null;
 try{
  const locale=new Intl.Locale(value.trim().replaceAll('_','-')).maximize();
  if(locale.language==='zh'&&locale.script==='Hans')return 'zh-CN';
  if(locale.language==='en'&&locale.script==='Latn')return 'en-US';
 }catch{}
 return null;
}
function resolveLocale(appLocale,environment={}){
 return normalizeLocale(environment.SUMA_LANGUAGE)||normalizeLocale(appLocale)||'zh-CN';
}
const messages=Object.freeze({
 startup_failed:Object.freeze({'zh-CN':'速码无法启动','en-US':'Suma could not start'}),
 service_stopped_title:Object.freeze({'zh-CN':'速码服务已停止','en-US':'Suma service stopped'}),
 service_stopped:Object.freeze({'zh-CN':'本地服务已停止，请重启速码以重新连接。','en-US':'The local service stopped. Restart Suma to reconnect.'}),
 shutdown_failed:Object.freeze({'zh-CN':'速码服务停止失败','en-US':'Suma service shutdown failed'}),
 missing_control:Object.freeze({'zh-CN':'缺少内置控制服务，请先构建桌面安装包。','en-US':'The bundled control service is missing. Build the desktop bundle first.'}),
 missing_web:Object.freeze({'zh-CN':'缺少网页资源，请先构建工作台。','en-US':'The web bundle is missing. Build the workbench first.'}),
 startup_exited:Object.freeze({'zh-CN':'内置控制服务在启动前退出','en-US':'Bundled control service exited before startup'}),
 startup_unhealthy:Object.freeze({'zh-CN':'内置控制服务未能就绪','en-US':'Bundled control service did not become healthy'}),
 termination_failed:Object.freeze({'zh-CN':'控制服务终止失败','en-US':'Control service termination failed'}),
 termination_timeout:Object.freeze({'zh-CN':'控制服务在强制终止后仍未退出','en-US':'Control service did not exit after forced termination'}),
 window_title:Object.freeze({'zh-CN':'速码','en-US':'Suma'}),
});
function message(locale,key){return messages[key]?.[normalizeLocale(locale)||'zh-CN']||key}
// 未知操作系统异常保留原始诊断；自有错误只翻译上下文，不伪造已覆盖的诊断。
function localError(locale,key,cause){
 const diagnostic=cause?.message;
 const error=new Error(message(locale,key)+(diagnostic?`\n\n${diagnostic}`:''),cause?{cause}:undefined);
 error.code=key;
 return error;
}
module.exports={normalizeLocale,resolveLocale,message,localError};
