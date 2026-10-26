#!/usr/bin/env node
import {inspectAdapters} from './catalog.js';
import {nativeOptions,nativeHelp,nativeDisplay} from './display.js';
const options=nativeOptions(process.argv.slice(2),process.env);
if(options.error){process.stderr.write(nativeDisplay(options.error,options.locale)!.localized_message+'\n');process.exit(2);}
if(options.action==='help'){process.stdout.write(nativeHelp(options.locale,true));process.exit(0);}
if(options.action==='version'){process.stdout.write('suma-agent-catalog 1.0.0\n');process.exit(0);}
if(options.action==='planning-probe'){process.stderr.write(nativeDisplay('native.cli.invalid_arguments',options.locale)!.localized_message+'\n');process.exit(2);}
process.stdout.write(JSON.stringify({items:await inspectAdapters(options.locale),execution_enabled:false},null,2)+'\n');
