# Local preparation diagnostics

Version 1.0.0, stage T211. [Contract](../../contracts/pool-preparation-diagnostics.md), [verification](../quality/pool-preparation-diagnostics-verification.md).

When local Pool preparation fails, the Runner reports a fixed reason, project-row index and field without disclosing private paths or contents. Expand **Preparation failed?**, paste the single JSON report and explicitly locate the problem. The form verifies its closed shape and the exact current input/output digests before focusing a row. Imported reports are historical, not live health checks; they never update configuration values or execute repairs.

Changing scope or inputs invalidates analysis. Reports produced before complete input was read can only match the output target and cannot locate a project row. Unconfirmed writes require checking the original file, without automatic overwrite or target replacement. Successful preparation keeps the original six-field receipt and all original authorization and dispatch gates.

The complete 31-requirement/P0–P7 platform remains active; this stage does not prove vendor/model execution or production acceptance.
