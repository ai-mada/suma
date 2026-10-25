import type { AriaLabelConfig } from '@xyflow/react';
import {tr as t, formatNumber} from './i18n';
import {formatLiteralValues} from './i18n/literal';

/** Read the controlled template without running interpolation or nesting on
 * raw node values. A replace callback inserts each value once, literally. */
export function flowHandleLabel(direction: 'input' | 'output', name: string, id: string): string {
  if(direction!=='input' && direction!=='output')return `${t('flow.handle')}: ${name} (${id})`;
  const result = formatLiteralValues(direction==='input'?'flow.handle.input':'flow.handle.output',{name,id});
  return result.formatted ? result.text : `${t('flow.handle')}: ${name} (${id})`;
}

/** The complete public 12.12.0 contract; TypeScript flags newly added labels. */
export function flowAriaLabelConfig(editable: boolean, movementNode?: () => string | undefined): AriaLabelConfig {
  const description = t(editable ? 'flow.node.edit' : 'flow.node.read');
  const directions: Record<string,string> = {
    up:t('flow.direction.up'),down:t('flow.direction.down'),left:t('flow.direction.left'),right:t('flow.direction.right'),
  };
  return {
    'node.a11yDescription.default':description,
    'node.a11yDescription.keyboardDisabled':description,
    // Official 12.12.0 supplies these coordinates before moveSelectedNodes().
    // Describe that starting position; do not infer the resulting coordinates.
    'node.a11yDescription.ariaLiveMessage':({direction,x,y}) => t('flow.node.moved',{
      id:movementNode?.() || t('flow.node.unidentified'),direction:directions[direction] || direction,x:formatNumber(x),y:formatNumber(y),
    }),
    'edge.a11yDescription.default':t(editable ? 'flow.edge.edit' : 'flow.edge.read'),
    'controls.ariaLabel':t('flow.controls'),
    'controls.zoomIn.ariaLabel':t('flow.zoom_in'),
    'controls.zoomOut.ariaLabel':t('flow.zoom_out'),
    'controls.fitView.ariaLabel':t('flow.fit_view'),
    'controls.interactive.ariaLabel':t('flow.interactive'),
    'minimap.ariaLabel':t('flow.minimap'),
    'handle.ariaLabel':t('flow.handle'),
  };
}
