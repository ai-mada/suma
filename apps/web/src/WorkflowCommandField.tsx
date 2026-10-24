import { useId } from "react";
import { tr as t, useLocale } from "./i18n";
import { Field } from "./ui";

export function WorkflowCommandField({ command, onChange, disabled = false, required = true }: {
  command?: string[];
  onChange: (command: string[]) => void;
  disabled?: boolean;
  required?: boolean;
}) {
  useLocale();
  const id = useId(), values = command || [], multiple = values.length > 1;
  const value = multiple ? JSON.stringify(values) : values[0] || "";
  // Match the shared command ID contract exactly, including trailing controls.
  const valid = (!required && values.length === 0) || (values.length === 1 && value.length >= 1 && value.length <= 100
    && /^[A-Za-z0-9]/.test(value) && !/[^A-Za-z0-9_.-]/.test(value));
  const error = multiple ? "development.workflow.command_multiple"
    : !value && required ? "development.workflow.command_required" : "development.workflow.command_invalid";
  const label = t(required ? "development.workflow.command_identifier" : "development.workflow.custom_execution_reference");
  return <>
    <Field label={label}>
      <input
        type="text"
        name="command_id"
        aria-label={label}
        aria-describedby={`${id}-help${valid ? "" : ` ${id}-error`}`}
        aria-invalid={!valid}
        required={required}
        pattern="[A-Za-z0-9][A-Za-z0-9_.\-]{0,99}"
        autoComplete="off"
        spellCheck={false}
        disabled={disabled}
        value={value}
        onChange={event => onChange(event.target.value === "" ? [] : [event.target.value])}
      />
      <small id={`${id}-help`}>{t(required ? "development.workflow.command_identifier_help" : "development.workflow.command_identifier_optional_help")}</small>
      {!valid && <small id={`${id}-error`} className="text-danger">{t(error)}</small>}
    </Field>
    {!valid && values.length > 0 && <pre className="output" aria-label={t("development.workflow.command_original_values")}>{JSON.stringify(values, null, 2)}</pre>}
  </>;
}
