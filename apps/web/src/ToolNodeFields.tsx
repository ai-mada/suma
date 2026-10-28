import { Trans } from "./i18n/Trans";
import { tr as t, useLocale } from "./i18n";
import type { WNode } from "./api";
import { ErrorNotice, Field } from "./ui";
import { toolBinding, type ToolProfile } from "./tool-model";
import { useToolList } from "./tool-hooks";
import {OctopExecutionFacts} from './OctopExecutionFacts';
export function ToolNodeFields({
  project,
  node,
  onChange,
}: {
  project: string;
  node: WNode;
  onChange: (patch: Partial<WNode>) => void;
}) { useLocale();
  const profiles = useToolList<ToolProfile>(project, "tool-profiles"),
    id =
      typeof node.config?.tool_profile_id === "string"
        ? node.config.tool_profile_id
        : "",
    availableProfiles = profiles.items?.filter(item => node.type === "test" ? item.profile.test_report_protocol === "suma-junit-report-v1" : !item.profile.test_report_protocol),
    current = availableProfiles?.find((item) => item.id === id);
  return (
    <section className="tool-binding">
      <Field
        label={t("development.node.fixed_external_tool_configuration")}
        hint={t("development.node.the_command_and_configuration_fingerprint_are_fixed")}
      >
        <select
          value={id}
          onChange={(event) =>
            onChange(
              toolBinding(
                node,
                event.target.value
                  ? availableProfiles?.find(
                      (item) => item.id === event.target.value,
                    )
                  : undefined,
              ),
            )
          }
        >
          <option value="">{t("development.node.no_binding_keep_manual_command_configuration")}</option>
          {id && !current && (
            <option value={id}>{t("development.node.binding_option",{id})}</option>
          )}
          {availableProfiles?.map((item) => (
            <option
              value={item.id}
              key={item.id}
              disabled={item.state === "revoked"}
            >
              {t(item.available?"development.node.tool_option":"development.node.tool_option_unavailable",{name:item.name,command:item.profile.command_id})}
            </option>
          ))}
        </select>
      </Field>
      {profiles.error && (
        <ErrorNotice
          error={profiles.error}
          onRetry={() => void profiles.read(profiles.error_action === "more")}
        />
      )}{" "}
      {profiles.next_cursor && (
        <button
          type="button"
          disabled={profiles.loading}
          onClick={() => void profiles.read(true)}
        >
          {t("development.node.load_more_actual_tool_configurations")}</button>
      )}
      {id && (
        <small>
          <Trans i18nKey="development.node.fixed_binding" values={{id,fingerprint:String(node.config?.tool_profile_sha256||t("development.node.missing_fingerprint_cannot_publish"))}} components={{id:<code/>,fingerprint:<code/>}}/>
        </small>
      )}
      {current && !current.available && (
        <p className="warning">
          {t("development.node.this_configuration_does_not_currently_meet_dispatch")}</p>
      )}
      {current?.profile.adapter==='octop'&&node.config?.tool_profile_sha256===current.profile.profile_sha256&&<OctopExecutionFacts capability={current.profile.octop_execution}/>}
      <a
        href={
          "?" +
          new URLSearchParams({
            project,
            view: "settings",
            sub: "tools",
            record: id,
          })
        }
      >
        {t("development.node.view_execution_tools_and_current_reason")}</a>
    </section>
  );
}
