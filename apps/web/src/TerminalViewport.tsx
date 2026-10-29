import {useEffect, useRef, useState} from 'react';
import type {Terminal} from '@xterm/xterm';
import type {FitAddon} from '@xterm/addon-fit';
import {tr, useLocale, UiError} from './i18n';
import {ErrorNotice} from './ui';
import type {TerminalWriter} from './terminal-hooks';
import '@xterm/xterm/css/xterm.css';

type Props = {inputEnabled: boolean; onReady: (writer?: TerminalWriter) => void;
  onInput: (bytes: Uint8Array) => void; onResize: (columns: number, rows: number) => void};
type TerminalRuntime = typeof import('@xterm/xterm').Terminal;

function localizeTerminal(runtime: TerminalRuntime) {
  runtime.strings.promptLabel = tr('terminal.input_label');
  runtime.strings.tooMuchOutput = tr('terminal.too_much_output');
}

/** xterm is loaded only after a user opens an existing/explicitly created
 * session. Parsing completion, rather than HTTP arrival, confirms consumption. */
export function TerminalViewport(props: Props) {
  const locale = useLocale(), host = useRef<HTMLDivElement>(null), terminal = useRef<Terminal | undefined>(undefined), fit = useRef<FitAddon | undefined>(undefined);
  const runtimeClass = useRef<TerminalRuntime | undefined>(undefined);
  const callbacks = useRef(props); callbacks.current = props;
  const [error, setError] = useState<unknown>();
  useEffect(() => {
    let disposed = false, observer: ResizeObserver | undefined, theme: MutationObserver | undefined;
    let resizeTimer: ReturnType<typeof setTimeout> | undefined;
    const releases: {dispose: () => void}[] = [], pending = new Set<(value: boolean) => void>();
    const applyTheme = () => {
      if (!terminal.current || !host.current) return;
      const tokens = getComputedStyle(host.current);
      terminal.current.options.theme = {background: tokens.getPropertyValue('--surface').trim(), foreground: tokens.getPropertyValue('--text').trim(),
        cursor: tokens.getPropertyValue('--accent').trim(), selectionBackground: tokens.getPropertyValue('--accent-soft').trim(),
        red: tokens.getPropertyValue('--red').trim(), green: tokens.getPropertyValue('--green').trim(), yellow: tokens.getPropertyValue('--amber').trim()};
    };
    void Promise.all([import('@xterm/xterm'), import('@xterm/addon-fit')]).then(([runtime, addon]) => {
      if (disposed || !host.current) return;
      runtimeClass.current = runtime.Terminal; localizeTerminal(runtime.Terminal);
      const view = new runtime.Terminal({cols: 80, rows: 24, convertEol: false, disableStdin: !callbacks.current.inputEnabled,
        fontFamily: '"Cascadia Code", "SFMono-Regular", Consolas, monospace', fontSize: 13, lineHeight: 1.25,
        cursorBlink: !window.matchMedia?.('(prefers-reduced-motion: reduce)').matches, screenReaderMode: true, scrollback: 2000});
      const sizing = new addon.FitAddon(); terminal.current = view; fit.current = sizing;
      view.loadAddon(sizing); view.open(host.current); applyTheme();
      view.textarea?.setAttribute('aria-label', tr('terminal.input_label'));
      releases.push(view.onData(data => {if (callbacks.current.inputEnabled) callbacks.current.onInput(new TextEncoder().encode(data));}));
      releases.push(view.onBinary(data => {if (callbacks.current.inputEnabled) callbacks.current.onInput(Uint8Array.from(data, character => character.charCodeAt(0) & 255));}));
      releases.push(view.onResize(size => {
        if (resizeTimer) clearTimeout(resizeTimer);
        resizeTimer = setTimeout(() => {if (!disposed) callbacks.current.onResize(size.cols, size.rows);}, 180);
      }));
      callbacks.current.onReady(bytes => new Promise<boolean>(resolve => {
        if (disposed || terminal.current !== view) {resolve(false); return;}
        pending.add(resolve); view.write(bytes, () => {pending.delete(resolve); resolve(!disposed && terminal.current === view);});
      }));
      observer = new ResizeObserver(() => {if (host.current?.clientWidth && host.current?.clientHeight) sizing.fit();});
      observer.observe(host.current); sizing.fit();
      theme = new MutationObserver(applyTheme); theme.observe(document.documentElement, {attributes: true, attributeFilter: ['data-theme']});
    }).catch(() => {if (!disposed) setError(new UiError('terminal.load_failed'));});
    return () => {
      disposed = true; callbacks.current.onReady(undefined); if (resizeTimer) clearTimeout(resizeTimer);
      observer?.disconnect(); theme?.disconnect(); releases.forEach(release => release.dispose());
      pending.forEach(resolve => resolve(false)); pending.clear(); terminal.current?.dispose(); terminal.current = undefined; fit.current = undefined; runtimeClass.current = undefined;
    };
  }, []);
  useEffect(() => {if (terminal.current) terminal.current.options.disableStdin = !props.inputEnabled;}, [props.inputEnabled]);
  useEffect(() => {
    if (runtimeClass.current) localizeTerminal(runtimeClass.current);
    const view = terminal.current;
    if (!view) return;
    view.textarea?.setAttribute('aria-label', tr('terminal.input_label'));
    // Public screenReaderMode rebuilds only xterm's accessibility mirror. This
    // clears a previous locale's stock overflow announcement without rewriting
    // PTY bytes, terminal scrollback, the session, or its output ACK cursor.
    if (view.options.screenReaderMode) {view.options.screenReaderMode = false; view.options.screenReaderMode = true;}
  }, [locale]);
  return <div className="terminal-viewport-wrap">{!!error && <ErrorNotice error={error}/>}<div className="terminal-viewport" ref={host}
    aria-label={tr('terminal.viewport_label')} data-terminal-viewport/></div>;
}
