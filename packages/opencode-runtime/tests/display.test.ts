import {test} from 'node:test';
import assert from 'node:assert/strict';
import {options,display,help} from '../src/display.js';
test('runtime human messages follow the current Chinese and English locale convention',()=>{
 assert.equal(options([],{}).locale,'zh-CN');assert.equal(options(['--language','en-US'],{SUMA_LANGUAGE:'zh-CN'}).locale,'en-US');assert.equal(options([],{SUMA_LANGUAGE:'en_US.UTF-8',LANG:'zh_CN'}).locale,'en-US');assert.equal(options(['--language=xx'],{}).error,true);
 for(const locale of ['zh-CN','en-US'] as const){assert.ok(help(locale).includes('opencode-sdk-v1'));assert.ok(help(locale).includes('JSONL'));assert.ok(display('probe_failed',locale).endsWith('\n'));}assert.match(display('probe_failed','zh-CN'),/探测失败/);assert.match(display('probe_failed','en-US'),/probe failed/);
});
