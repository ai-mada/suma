import {inspectSourceJSON,MAX_INPUT,MAX_OUTPUT} from './index.mjs';
// stdin/stdout are the sole data surfaces. No project directory, URL or secret
// is accepted, and this is not represented as an operating-system sandbox.
let size=0;const chunks=[];
try{
 for await(const chunk of process.stdin){size+=chunk.length;if(size>MAX_INPUT)throw Error('source_inspector_input_limit');chunks.push(chunk)}
 const report=inspectSourceJSON(new TextDecoder('utf-8',{fatal:true}).decode(Buffer.concat(chunks))),output=JSON.stringify(report);if(Buffer.byteLength(output)>MAX_OUTPUT)throw Error('source_inspector_report_limit');process.stdout.write(output+'\n');
}catch(error){const code=error instanceof Error&&/^source_inspector_(invalid_input|input_limit|report_limit)$/.test(error.message)?error.message:'source_inspector_invalid_input';process.stderr.write(code+'\n');process.exitCode=2}
