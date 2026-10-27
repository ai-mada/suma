import {useCallback, useEffect, useMemo, useState} from 'react';
import type {ExpertTeamRole, ModelSelection} from '../../../packages/contracts/src';
import {ModelExecutionPanel, type ModelChoice} from './ModelExecutionPanel';
import {formatNumber, tr, useLocale} from './i18n';

export type ExpertModelTask = {taskID: string; title: string; role: ExpertTeamRole};
export type ExpertModelChoices = {scope: string; ready: boolean; selections: Record<string, ModelSelection | undefined>};
const t = (key: string, values?: Record<string, unknown>) => tr('expert-model.' + key, values);
const roleScope = (role: ExpertTeamRole) => JSON.stringify([role.key, role.tool_profile_id, role.tool_profile_version, role.tool_profile_sha256]);
const taskScope = (task: ExpertModelTask) => JSON.stringify([task.taskID, roleScope(task.role)]);
/** Consumers compare the complete current scope synchronously, before submitting.
 * A choice for another role, version, task set or project is never inherited. */
export const expertModelScope = (project: string, tasks: ExpertModelTask[]) => JSON.stringify([project, tasks.map(taskScope)]);

export function ExpertModelSelections({project, tasks, disabled, onChange}: {
  project: string; tasks: ExpertModelTask[]; disabled: boolean;
  onChange: (value: ExpertModelChoices) => void;
}) {
  const scope = expertModelScope(project, tasks);
  return <ScopedSelections key={scope} project={project} tasks={tasks} scope={scope} disabled={disabled} onChange={onChange}/>;
}
function ScopedSelections({project, tasks, scope, disabled, onChange}: {
  project: string; tasks: ExpertModelTask[]; scope: string; disabled: boolean;
  onChange: (value: ExpertModelChoices) => void;
}) {
  useLocale();
  const [shared, setShared] = useState<Record<string, ModelChoice>>({});
  const [overrides, setOverrides] = useState<Record<string, boolean>>({});
  const [individual, setIndividual] = useState<Record<string, ModelChoice>>({});
  const groups = useMemo(() => {
    const result = new Map<string, ExpertModelTask[]>();
    for (const task of tasks) {
      const key = roleScope(task.role);
      result.set(key, [...(result.get(key) || []), task]);
    }
    return [...result.entries()];
  }, [scope]);
  useEffect(() => {
    const selections: ExpertModelChoices['selections'] = {};
    const ready = tasks.length > 0 && tasks.every(task => {
      const choice = overrides[taskScope(task)] ? individual[taskScope(task)] : shared[roleScope(task.role)];
      selections[task.taskID] = choice?.selection;
      return choice?.ready === true;
    });
    onChange({scope, ready, selections});
  }, [scope, shared, individual, overrides, onChange]);
  const sharedChoice = useCallback((key: string, value: ModelChoice) => setShared(current => ({...current, [key]: value})), []);
  const taskChoice = useCallback((key: string, value: ModelChoice) => setIndividual(current => ({...current, [key]: value})), []);
  return <section className="expert-model-selections" aria-label={t('title')}>
    <h4>{t('title')}</h4><p className="team-dispatch-muted">{t('intro')}</p>
    {groups.map(([key, group]) => {
      const role = group[0].role;
      return <section className="team-dispatch-callout" data-expert-model-role={role.key} key={key}>
        <strong>{role.title} · {t('tasks', {count: formatNumber(group.length)})}</strong>
        <Choice project={project} role={role} disabled={disabled} choiceKey={key} onChange={sharedChoice}/>
        {group.length > 1 && <details><summary>{t('individual')}</summary>
          <p className="team-dispatch-muted">{t('individual_help')}</p>
          {group.map(task => <div key={taskScope(task)} data-expert-model-task={task.taskID}>
            <label className="team-dispatch-check"><input type="checkbox" disabled={disabled}
              checked={!!overrides[taskScope(task)]} aria-label={t('override', {title: task.title})}
              onChange={event => setOverrides(current => ({...current, [taskScope(task)]: event.target.checked}))}/>
              <span>{task.title}</span>
            </label>
            {overrides[taskScope(task)] && <Choice project={project} role={role} disabled={disabled}
              choiceKey={taskScope(task)} onChange={taskChoice}/>}
          </div>)}
        </details>}
      </section>;
    })}
    <p className="team-dispatch-muted">{t('budget_help')}</p>
  </section>;
}
function Choice({project, role, disabled, choiceKey, onChange}: {
  project: string; role: ExpertTeamRole; disabled: boolean; choiceKey: string;
  onChange: (key: string, value: ModelChoice) => void;
}) {
  const changed = useCallback((value: ModelChoice) => onChange(choiceKey, value), [choiceKey, onChange]);
  // Discovery uses the authoritative current tool. A non-native tool stays
  // fixed only after a scoped unsupported response; errors remain blocking.
  return <ModelExecutionPanel scope={{project, profile: role.tool_profile_id, version: role.tool_profile_version, sha256: role.tool_profile_sha256}}
    supported={false} discover disabled={disabled} onChange={changed}/>;
}
