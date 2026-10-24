#!/usr/bin/env node
import {inspectAdapters} from './catalog.js';
process.stdout.write(JSON.stringify({items:await inspectAdapters(),execution_enabled:false},null,2)+'\n');
