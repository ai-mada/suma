# Execution tool discovery

Open **Tool management → Execution tools**, register a node and start its configured Runner locally using the issued credential. Selecting the node in the registration form reads its latest report automatically. A sole available tool is selected with an editable suggested name. Review and register it paused, then enable it explicitly. Discovery never installs tools or starts work.

An unreported node needs its Runner started with the current project credential. A stale report needs connection/credential checks and a new Runner report. A fresh empty report needs local installation/configuration checks. Unavailable entries retain their actual reasons. Read and authorization failures offer explicit retry; unconfirmed current authority blocks registration.

For remote or offline setup, expand **Paste or import a JSON inventory**. Run `suma-runner --describe-tools` locally with the same configured `--commands` file and approved execution policy. Paste the complete output or import a saved JSON file. Files are read locally; paths, arguments, environment values and vendor login files stay on the node. The Web parser accepts at most128 observations and256KiB, rejects excess input and never truncates it into a successful result.

Manual inventories and names survive delayed reads. Explicitly replace an edited inventory with the same fresh report when needed. Changing nodes clears automatically sourced configurations. Reports expire60 seconds after their original database receive time; a refresh only reads already-reported facts. Configuration, dispatch eligibility, model authentication, execution and formal acceptance remain separate.
