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
 desktop_port_invalid:Object.freeze({'zh-CN':'本机端口配置无效。SUMA_DESKTOP_PORT 需为 1024–65535 的整数，默认 4319。','en-US':'Invalid local port. SUMA_DESKTOP_PORT must be an integer from 1024 to 65535; the default is 4319.'}),
 desktop_port_unavailable:Object.freeze({'zh-CN':'本机端口已被占用或不可使用。请关闭占用程序，或配置其他 SUMA_DESKTOP_PORT；速码不会连接未知服务或自动换端口。','en-US':'The local port is occupied or unavailable. Close its owner or configure another SUMA_DESKTOP_PORT. Suma will not attach to an unknown service or silently change ports.'}),
 startup_failed:Object.freeze({'zh-CN':'速码无法启动','en-US':'Suma could not start'}),
 service_stopped_title:Object.freeze({'zh-CN':'速码服务已停止','en-US':'Suma service stopped'}),
 service_stopped:Object.freeze({'zh-CN':'本地服务已停止，请重启速码以重新连接。','en-US':'The local service stopped. Restart Suma to reconnect.'}),
 shutdown_failed:Object.freeze({'zh-CN':'速码服务停止失败','en-US':'Suma service shutdown failed'}),
 missing_control:Object.freeze({'zh-CN':'缺少内置控制服务，请先构建桌面安装包。','en-US':'The bundled control service is missing. Build the desktop bundle first.'}),
 missing_web:Object.freeze({'zh-CN':'缺少网页资源，请先构建工作台。','en-US':'The web bundle is missing. Build the workbench first.'}),
 missing_source_inspector:Object.freeze({'zh-CN':'缺少内置源码观察器，请重新准备桌面资源或安装完整安装包。','en-US':'The bundled source inspector is missing. Prepare desktop resources again or install the complete package.'}),
 startup_exited:Object.freeze({'zh-CN':'内置控制服务在启动前退出','en-US':'Bundled control service exited before startup'}),
 startup_database_incompatible:Object.freeze({'zh-CN':'现有数据库与当前版本不兼容。原数据已保留，请备份后使用单独的新数据目录；旧数据不会自动迁移。','en-US':'The existing database is incompatible with this version. Your data has been preserved. Back it up and use a separate new data directory; existing data is not migrated automatically.'}),
 startup_unhealthy:Object.freeze({'zh-CN':'内置控制服务未能就绪','en-US':'Bundled control service did not become healthy'}),
 termination_failed:Object.freeze({'zh-CN':'控制服务终止失败','en-US':'Control service termination failed'}),
 termination_timeout:Object.freeze({'zh-CN':'控制服务在强制终止后仍未退出','en-US':'Control service did not exit after forced termination'}),
 window_title:Object.freeze({'zh-CN':'速码','en-US':'Suma'}),
 departure_title:Object.freeze({'zh-CN':'离开当前工作台？','en-US':'Leave this workbench?'}),
 departure_message:Object.freeze({'zh-CN':'当前页面有尚未完成的操作或未保存的内容。','en-US':'This page has an unfinished operation or unsaved changes.'}),
 departure_detail:Object.freeze({'zh-CN':'留下可继续核对原操作。离开页面不会取消任务或确认执行结果；关闭应用仍按原流程收尾，并保留待核对记录。','en-US':'Stay to continue checking the original operation. Leaving the page does not cancel tasks or confirm results. Closing the app follows its normal shutdown and preserves records awaiting reconciliation.'}),
 departure_stay:Object.freeze({'zh-CN':'留下','en-US':'Stay'}),
 departure_leave:Object.freeze({'zh-CN':'离开','en-US':'Leave'}),
 workspace_choose_title:Object.freeze({'zh-CN':'选择当前项目的工作目录','en-US':'Choose this project’s working directory'}),
 workspace_choose_button:Object.freeze({'zh-CN':'选择目录','en-US':'Choose directory'}),
 workspace_key_failed:Object.freeze({'zh-CN':'桌面工作区授权密钥不可用，请检查本地应用数据目录。','en-US':'The desktop workspace key is unavailable. Check the local app data directory.'}),
 desktop_data_failed:Object.freeze({'zh-CN':'桌面本地数据目录不可用。','en-US':'The local desktop data directory is unavailable.'}),
 desktop_configuration_invalid:Object.freeze({'zh-CN':'桌面验证配置无效，请使用本次验证目录内的完整本地路径。','en-US':'The desktop validation configuration is invalid. Use complete local paths inside this validation directory.'}),
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
