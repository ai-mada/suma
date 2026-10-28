# Third-party notices and dependency inventory

This Suma adapter uses published Python distributions through public APIs. Vendor implementations, built-in skills, office templates, and user data are not copied into the zipapp source. The managed Python environment retains each distribution's original license files.

## Direct dependencies

| Distribution | Exact version | License from installed official release | Authoritative source |
|---|---|---|---|
| octop-harness | 1.0.5 | MIT; Copyright (c) 2026 Octop | [source fixed for review](https://github.com/TencentCloud/octop-harness/tree/68b5423bb80ecef7fca57ba5a37d9a2bdcbf12d2), [PyPI](https://pypi.org/project/octop-harness/1.0.5/) |
| agent-client-protocol | 0.12.1 | Apache License, Version 2.0 | [official Python SDK](https://github.com/agentclientprotocol/python-sdk), [PyPI](https://pypi.org/project/agent-client-protocol/0.12.1/) |
| mcp | 1.30.0 | MIT; Copyright (c) 2024 Anthropic, PBC | [official Python SDK](https://github.com/modelcontextprotocol/python-sdk), [PyPI](https://pypi.org/project/mcp/1.30.0/) |
| jsonschema | 4.26.0 | MIT; Copyright (c) 2013 Julian Berman | [official source](https://github.com/python-jsonschema/jsonschema), [PyPI](https://pypi.org/project/jsonschema/4.26.0/) |

Release license files, relative to the managed environment's `site-packages`:

- `octop_harness-1.0.5.dist-info/licenses/LICENSE`
- `agent_client_protocol-0.12.1.dist-info/licenses/LICENSE`
- `mcp-1.30.0.dist-info/licenses/LICENSE`
- `jsonschema-4.26.0.dist-info/licenses/COPYING`

The wheel of Octop Harness also includes separately licensed built-in office skills. This adapter does not seed, enable or copy those skills. That fact does not remove license obligations when redistributing the complete installed wheel or environment. Preserve its original files and notices.

## Bill of materials and deployment boundary

The complete current inventory is [requirements.lock](requirements.lock): **96 exact distribution versions**, resolved and installed using Windows Python 3.13.3. `pywin32==312` has an explicit `sys_platform == "win32"` marker. It is not requested on macOS or Linux. Cross-platform installation, native wheel availability and behavior have not been verified by this stage.

The lock freezes versions, rather than artifact hashes for all operating systems. The zipapp build records every installed distribution/version in both `suma-octop.manifest.json` and embedded `suma_octop/dependency-lock.json`; runtime probe verifies these versions before executing. The Go profile pins the Python entry, zipapp and manifest. This is not a claim that a version list attests every third-party byte. Administrators must protect and distribute the matching environment and original license notices. Additional OS packaging/signing and vendor-account validation remain separate acceptance work.

Reviewed official wheel digests:

- `octop_harness-1.0.5-py3-none-any.whl`: SHA256 `40a27c3ba62eb7cb6e555ed73f8e888b668cfbf447fc7ed4229e200f780ae1b4`
- `agent_client_protocol-0.12.1-py3-none-any.whl`: SHA256 `aaf3cc301ed87d9e2c4ccb91adfe1cd58784355f6ce7e513f50e7ca2391862ef`

## MIT notice

The MIT-licensed components above include the following permission and warranty terms. Their respective copyright lines remain as stated in the table and official distribution files.

> Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:
>
> The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.
>
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

The complete [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0) is preserved in the ACP distribution's `LICENSE`. Preserve that file and any applicable NOTICE when redistributing the environment; this short inventory does not replace its terms.
