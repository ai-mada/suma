/// <reference types="vite/client" />
import zhFoundation from './zh-CN.json';
import enFoundation from './en-US.json';
import {mergeLocaleCatalogs} from '../catalog-modules';
// Eager local imports keep language switching available offline and during
// a disconnected operation; no remote catalog upload or runtime fetch is used.
const domains=import.meta.glob('./domains/*.json',{eager:true,import:'default'});
export const zh=mergeLocaleCatalogs(zhFoundation,domains,'zh-CN');
export const en=mergeLocaleCatalogs(enFoundation,domains,'en-US');
