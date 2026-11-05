export const managementAreas = [
  {id:'projects',start:'projects',tabs:[['projects','management.project_directory'],['project','shell.tabs.project'],['members','shell.tabs.resources'],['project-access','shell.tabs.project_access']]},
  {id:'teams',start:'teams',tabs:[['teams','shell.tabs.teams']]},
  {id:'models',start:'models',tabs:[['models','management.model_directory']]},
  {id:'tools',start:'tools',tabs:[['tools','shell.tabs.tools'],['project-execution','shell.tabs.execution_resources'],['scheduling','shell.tabs.scheduling'],['execution-pools','shell.tabs.pools']]},
  {id:'billing',start:'usage',tabs:[['usage','management.usage_rates_quotas']]},
  {id:'organization',start:'organization',tabs:[['organization','shell.tabs.organization'],['work-calendars','organization-work-calendar.title'],['preferences','preferences.title'],['capabilities','shell.tabs.integrations'],['tokens','shell.tabs.tokens']]},
] as const;
export type ManagementArea = typeof managementAreas[number];
export function managementArea(sub:string):ManagementArea {
  return managementAreas.find(area=>area.tabs.some(([id])=>id===sub))||managementAreas[5];
}
