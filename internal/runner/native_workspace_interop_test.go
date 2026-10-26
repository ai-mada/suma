package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This optional integration gate uses the actual installed official TS SDK and
// the same Go server as Execute. Go-only CI reports an explicit skip when its
// Node dependency is not installed; it never substitutes a protocol fixture.
func TestNativeWorkspaceOfficialTypeScriptSDKInterop(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("cross-SDK interoperability was not run: Node.js is not installed")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	module := filepath.Join(repository, "packages", "agent-runtime", "node_modules", "@modelcontextprotocol", "client", "dist", "index.mjs")
	if _, err = os.Stat(module); os.IsNotExist(err) {
		t.Skip("cross-SDK interoperability was not run: the official TypeScript client dependency is not installed")
	} else if err != nil {
		t.Fatal(err)
	}
	modulePath := filepath.ToSlash(module)
	if !strings.HasPrefix(modulePath, "/") {
		modulePath = "/" + modulePath
	}
	moduleURL := (&url.URL{Scheme: "file", Path: modulePath}).String()
	n, _, _ := nativeFixture(t, nil, nil)
	bridge, err := n.StartMCP()
	if err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(t.TempDir(), "probe.mjs")
	if err = os.WriteFile(probe, []byte(nativeWorkspaceInteropProbe), 0600); err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]any{"bridge": bridge, "client_module_url": moduleURL})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, probe)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env, _, err = commandEnvironment(CommandSpec{})
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr := newOutput(16<<10, []string{bridge.Token}), newOutput(8<<10, []string{bridge.Token})
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	var report struct {
		Connected bool `json:"connected"`
		Catalog   bool `json:"catalog"`
		Traffic   []struct {
			Method string `json:"method"`
			Status int    `json:"status"`
		} `json:"traffic"`
		NullRequiredPaths []string `json:"null_required_paths"`
		Issues            []struct {
			Code string `json:"code"`
			Path []any  `json:"path"`
		} `json:"issues"`
	}
	if stdout.truncated || stderr.truncated || json.Unmarshal([]byte(stdout.String()), &report) != nil {
		t.Fatal("cross-SDK probe did not return its bounded safe report")
	}
	safe, _ := json.Marshal(report)
	t.Logf("official TypeScript SDK interoperability: %s", safe)
	if runErr != nil || !report.Connected || !report.Catalog || len(report.NullRequiredPaths) != 0 {
		t.Fatalf("official TypeScript SDK connect/listTools failed (process_success=%t)", runErr == nil)
	}
}

const nativeWorkspaceInteropProbe = `// Diagnostic-only Go/official-TypeScript-SDK interoperation probe.
// Input contains a live secret; never print input, headers, bodies or errors.
const report={connected:false,catalog:false,traffic:[],null_required_paths:[],issues:[]};
let client;
const timeout=AbortSignal.timeout(10000);
function nullRequired(value,path){
 if(!value||typeof value!=='object')return;
 if(!Array.isArray(value)&&Object.hasOwn(value,'required')&&value.required===null)report.null_required_paths.push(path.concat('required').join('.'));
 for(const [key,item] of Object.entries(value))if(/^[A-Za-z0-9_.-]{1,100}$/.test(key))nullRequired(item,path.concat(key));
}
function safeIssues(issues){
 return issues.map(issue=>({code:typeof issue.code==='string'&&/^[a-z_]{1,80}$/.test(issue.code)?issue.code:'invalid_schema',path:Array.isArray(issue.path)?issue.path.filter(part=>typeof part==='number'||typeof part==='string'&&/^[A-Za-z0-9_.-]{1,100}$/.test(part)):[]}));
}
try{
 const chunks=[];let size=0;for await(const chunk of process.stdin){size+=chunk.length;if(size>16384)throw Error();chunks.push(chunk);}
 const input=JSON.parse(Buffer.concat(chunks).toString('utf8'));
 if(typeof input.client_module_url!=='string'||!input.client_module_url.startsWith('file:'))throw Error();
 const sdk=await import(input.client_module_url),bridge=input.bridge;
 const fetchProbe=async(url,init)=>{
  if(String(url)!==bridge.endpoint)throw Error();
  let method=init?.method||'GET';if(typeof init?.body==='string')try{method=JSON.parse(init.body).method||method;}catch{}
  const response=await fetch(url,{...init,headers:{...Object.fromEntries(new Headers(init?.headers)),Authorization:'Bearer '+bridge.token},redirect:'error',signal:AbortSignal.any([timeout,...(init?.signal?[init.signal]:[])])});
  report.traffic.push({method,status:response.status});
  if(method==='tools/list'){
   try{
    const message=await response.clone().json();
    if(Array.isArray(message.result?.tools))message.result.tools.forEach((tool,index)=>nullRequired(tool.inputSchema,['result','tools',index,'inputSchema']));
    const validation=sdk.specTypeSchemas.ListToolsResult.safeParse(message.result);
    if(!validation.success)report.issues.push(...safeIssues(validation.error.issues));
   }catch{}
  }
  return response;
 };
 client=new sdk.Client({name:'suma-sdk-interop-probe',version:'1'},{defaultCacheTtlMs:0,listMaxPages:1});
 const transport=new sdk.StreamableHTTPClientTransport(new URL(bridge.endpoint),{fetch:fetchProbe,requestInit:{redirect:'error'},onInsufficientScope:'throw',reconnectionOptions:{maxRetries:0,initialReconnectionDelay:1000,maxReconnectionDelay:1000,reconnectionDelayGrowFactor:1}});
 await client.connect(transport,{signal:timeout});report.connected=true;
 const result=await client.listTools(undefined,{signal:timeout});
 const actual=result.tools.map(tool=>tool.name).sort(),expected=['list','read','write','remove','check','edit'].map(name=>'suma_workspace_'+name).sort();
 report.catalog=JSON.stringify(actual)===JSON.stringify(expected);
}catch{}
finally{try{await client?.close();}catch{}process.stdout.write(JSON.stringify(report)+'\n');process.exitCode=report.connected&&report.catalog?0:1;}

`
