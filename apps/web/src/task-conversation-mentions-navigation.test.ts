import {expect, it} from 'vitest';
import {locateWorkbenchTask, parseWorkbenchRoute, resetTaskCollectionScope} from './navigation';

it('locates one original discussion in the existing task workspace and respects the leave guard', () => {
  let route = parseWorkbenchRoute('?project=p&view=development&sub=inbox');
  const update = (change: (value: typeof route) => typeof route) => {route = change(route);};
  expect(locateWorkbenchTask('task-a', () => false, update, 'message-a')).toBe(false);
  expect(route.sub).toBe('inbox');
  expect(locateWorkbenchTask('task-a', () => true, update, 'message-a')).toBe(true);
  expect(route).toMatchObject({sub:'ide', panel:'workbench-task:task-a',conversation_message:'message-a',record:'',run:''});
  expect(locateWorkbenchTask('task-b', () => true, update)).toBe(true);
  expect(route.conversation_message).toBeUndefined();
  locateWorkbenchTask('', () => true, update); expect(route.panel).toBe(''); expect(route.conversation_message).toBeUndefined();
});
it('accepts only one exact in-workspace message locator and clears project-bound references', () => {
  const base='?project=p&view=development&sub=ide&panel=workbench-task:task-a';
  expect(parseWorkbenchRoute(base+'&conversation_message=message-a').conversation_message).toBe('message-a');
  for(const suffix of ['../private','message-a&conversation_message=message-b'])expect(parseWorkbenchRoute(base+'&conversation_message='+suffix).conversation_message).toBeUndefined();
  expect(parseWorkbenchRoute('?project=p&view=development&sub=inbox&conversation_message=message-a').conversation_message).toBeUndefined();
  expect(resetTaskCollectionScope(parseWorkbenchRoute(base+'&conversation_message=message-a')).conversation_message).toBeUndefined();
});
