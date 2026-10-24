import {describe,it,expect} from 'vitest';
import {mergeLocaleCatalogs} from './catalog-modules';
const modules={
  './domains/work.zh-CN.json':{'work.title':'工作'},
  './domains/work.en-US.json':{'work.title':'Work'},
};
describe('domain catalog assembly',()=>{
  it('assembles exactly one locale without changing its input objects',()=>{
    const base=Object.freeze({'base.title':'基础'});
    expect(mergeLocaleCatalogs(base,modules,'zh-CN')).toEqual({'base.title':'基础','work.title':'工作'});
    expect(mergeLocaleCatalogs({},modules,'en-US')).toEqual({'work.title':'Work'});
    expect(base).toEqual({'base.title':'基础'});
  });
  it('rejects overrides, unpaired catalogs and keys outside the owning domain',()=>{
    expect(()=>mergeLocaleCatalogs({'work.title':'旧'},modules,'zh-CN')).toThrow(/duplicate/);
    expect(()=>mergeLocaleCatalogs({}, {'./domains/work.zh-CN.json':{}},'zh-CN')).toThrow(/pair/);
    expect(()=>mergeLocaleCatalogs({}, {'./domains/work.zh-CN.json':{'other.title':'错误'},'./domains/work.en-US.json':{'other.title':'No'}},'en-US')).toThrow(/namespace/);
  });
  it('requires matching language keys and own nonempty string values',()=>{
    expect(()=>mergeLocaleCatalogs({}, {...modules,'./domains/work.en-US.json':{}},'en-US')).toThrow(/key/);
    expect(()=>mergeLocaleCatalogs({}, {...modules,'./domains/work.en-US.json':{'work.title':0}},'en-US')).toThrow(/string/);
    expect(()=>mergeLocaleCatalogs({}, {...modules,'./domains/work.en-US.json':{'work.title':' '}},'en-US')).toThrow(/string/);
  });
});
