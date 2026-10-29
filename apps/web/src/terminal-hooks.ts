import {useCallback, useEffect, useRef, useState} from 'react';
import type {WorkspaceTerminalSession, WorkspaceTerminalCommandReceipt} from '../../../packages/contracts/src';
import {ApiError, operation, query} from './api';
import {useApi} from './api-context';
import {UiError} from './i18n';
import {terminalBase64, terminalBytes, terminalEnded, terminalSHA, validTerminalCommand, validTerminalOutput, validTerminalSession} from './terminal-model';

export type TerminalWriter = (bytes: Uint8Array) => Promise<boolean>;
export type TerminalConnection = 'detached' | 'connecting' | 'connected' | 'retrying' | 'forbidden';
type Props = {project: string; task: string; session?: WorkspaceTerminalSession; active: boolean; writer?: TerminalWriter;
  controlBlocked: boolean; onSession: (session: WorkspaceTerminalSession) => void; onDenied: () => void;
  sendControl: (body: Record<string, unknown>) => Promise<WorkspaceTerminalCommandReceipt | undefined>};

/** One serialized command lane shares the current CAS version. Output reads
 * remain independent. No shell bytes enter browser storage or write recovery. */
export function useTerminalConnection(props: Props) {
  const api = useApi(), [connection, setConnection] = useState<TerminalConnection>('detached'), [error, setError] = useState<unknown>();
  const [gap, setGap] = useState(false), [inputIssue, setInputIssue] = useState<'unknown' | 'rejected' | undefined>();
  const [lastInput, setLastInput] = useState<string>();
  const callbacks = useRef(props); callbacks.current = props;
  const latest = useRef(props.session), cursor = useRef(0), ackWanted = useRef(0), alive = useRef(true), owner = useRef(api);
  const commands = useRef<Promise<unknown>>(Promise.resolve()), rendering = useRef<Promise<unknown>>(Promise.resolve());
  const identity = useRef({api, session: props.session?.id || ''}), connectedSession = useRef('');
  const inputLatch = useRef(false), state = useRef(connection); state.current = connection; owner.current = api;
  if (identity.current.api !== api || identity.current.session !== (props.session?.id || '')) {
    identity.current = {api, session: props.session?.id || ''}; cursor.current = 0; ackWanted.current = 0;
    latest.current = props.session; inputLatch.current = false; connectedSession.current = '';
  }
  if (props.session && (!latest.current || props.session.id !== latest.current.id || props.session.version >= latest.current.version)) latest.current = props.session;
  const sessionID = props.session?.id || '';
  const acceptsInput = props.active && connectedSession.current === sessionID && connection === 'connected' && !inputIssue && !props.controlBlocked && !!props.session?.allowed_actions.includes('input');
  const acceptRef = useRef(acceptsInput); acceptRef.current = acceptsInput;
  useEffect(() => {alive.current = true; return () => {alive.current = false; inputLatch.current = true;};}, []);
  useEffect(() => {setGap(false); setInputIssue(undefined); setLastInput(undefined); setError(undefined);}, [api, sessionID]);
  const exclusive = useCallback(<T,>(work: () => Promise<T>): Promise<T> => {
    const result = commands.current.then(work, work); commands.current = result.catch(() => undefined); return result;
  }, []);
  const current = () => alive.current && owner.current === api && latest.current?.id === sessionID;
  function accept(session: WorkspaceTerminalSession) {
    if (!current() || session.id !== sessionID || latest.current && session.version < latest.current.version) return;
    latest.current = session; callbacks.current.onSession(session);
  }
  async function readLatest(signal?: AbortSignal) {
    const value = await api<unknown>('/workspace/terminal-sessions/' + encodeURIComponent(sessionID) + query(props.project), undefined, 'GET', undefined, signal);
    if (!validTerminalSession(value, props.project, props.task) || value.id !== sessionID) throw new UiError('terminal.invalid_read');
    accept(value); return latest.current!;
  }
  function failed(problem: unknown) {
    if (!current()) return;
    setError(problem);
    if (problem instanceof ApiError && [401, 403, 404].includes(problem.status)) {
      setConnection('forbidden'); inputLatch.current = true; callbacks.current.onDenied();
    }
  }
  const control = useCallback((action: 'ack' | 'resize' | 'stop', fields: Record<string, unknown> = {}) => exclusive(async () => {
    if (!current() || callbacks.current.controlBlocked) return;
    try {
      const session = await readLatest();
      if (!session.allowed_actions.includes(action)) return;
      const receipt = await callbacks.current.sendControl({project_id: props.project, expected_version: session.version,
        control_epoch: session.control_epoch, action, ...fields});
      if (receipt) {accept(receipt.session); return receipt;}
    } catch (problem) {failed(problem);}
  }), [api, sessionID, props.project, props.task, exclusive]);

  const input = useCallback((bytes: Uint8Array) => {
    if (!bytes.length) return;
    if (bytes.length > 8192 || !acceptRef.current || inputLatch.current) {
      inputLatch.current = true; setInputIssue('rejected'); return;
    }
    const payload = bytes.slice();
    void exclusive(async () => {
      if (!current() || !callbacks.current.active || callbacks.current.controlBlocked || inputLatch.current || state.current !== 'connected') {
        if (current()) {inputLatch.current = true; setInputIssue('rejected');} return;
      }
      let sent = false;
      try {
        const session = await readLatest();
        if (!current() || !session.allowed_actions.includes('input')) {inputLatch.current = true; setInputIssue('rejected'); return;}
        const inputID = operation(), sha = await terminalSHA(payload);
        if (!current() || !callbacks.current.active) {inputLatch.current = true; setInputIssue('rejected'); return;}
        sent = true;
        const value = await api<unknown>('/workspace/terminal-sessions/' + encodeURIComponent(sessionID) + '/commands', {
          project_id: props.project, expected_version: session.version, control_epoch: session.control_epoch, action: 'input',
          input_id: inputID, input_sequence: session.next_input_sequence, data_base64: terminalBase64(payload), idempotency_key: operation()});
        if (!current()) return;
        if (!validTerminalCommand(value, props.project, props.task, sessionID) || !value.input || value.input.input_id !== inputID ||
          value.input.sequence !== session.next_input_sequence || value.input.sha256 !== sha || value.input.bytes_written > payload.length ||
          value.input.state === 'delivered' && value.input.bytes_written !== payload.length) throw new UiError('terminal.invalid_read');
        accept(value.session); setLastInput(value.input.state);
        if (value.input.state === 'delivery_unknown') {inputLatch.current = true; setInputIssue('unknown');}
      } catch (problem) {
        if (!current()) return;
        inputLatch.current = true;
        setInputIssue(sent && (!(problem instanceof ApiError) || problem.status === 0 || problem.status >= 500) ? 'unknown' : 'rejected');
        failed(problem);
      } finally {payload.fill(0);}
    });
  }, [api, sessionID, props.project, props.task, exclusive]);

  useEffect(() => {
    if (!sessionID || !props.active || !props.writer) {setConnection('detached'); return;}
    let stopped = false, timer: ReturnType<typeof setTimeout> | undefined, failures = 0;
    const abort = new AbortController(); setConnection('connecting');
    async function poll() {
      if (stopped || !current()) return;
      try {
        const session = await readLatest(abort.signal), after = cursor.current;
        const page = await api<unknown>('/workspace/terminal-sessions/' + encodeURIComponent(sessionID) + '/output' + query(props.project) + '&after=' + after,
          undefined, 'GET', undefined, abort.signal);
        if (stopped || !current()) return;
        if (!validTerminalOutput(page, session, after)) throw new UiError('terminal.invalid_read');
        if (page.gap) setGap(true);
        for (const chunk of page.chunks) {
          const render = rendering.current.then(async () => {
            if (!current() || chunk.sequence <= cursor.current) return;
            const bytes = terminalBytes(chunk.data_base64)!;
            const consumed = await callbacks.current.writer?.(bytes);
            // A collapse may finish parsing its last chunk. Keep that real
            // cursor, but do not send an ACK after an owner/session change.
            if (consumed && current()) {cursor.current = chunk.sequence; ackWanted.current = chunk.sequence;}
          });
          rendering.current = render.catch(() => undefined); await render;
          if (stopped || !current()) return;
        }
        if (ackWanted.current > (latest.current?.output_ack_sequence || 0) && latest.current?.allowed_actions.includes('ack') && !callbacks.current.controlBlocked) {
          await control('ack', {output_sequence: ackWanted.current});
        }
        if (stopped || !current()) return;
        failures = 0; setError(undefined); connectedSession.current = sessionID; setConnection('connected');
        if (!terminalEnded(latest.current!) || page.last_sequence > cursor.current) timer = setTimeout(() => void poll(), page.chunks.length ? 150 : 500);
      } catch (problem) {
        if (stopped || abort.signal.aborted || !current()) return;
        failed(problem);
        if (problem instanceof ApiError && [401, 403, 404].includes(problem.status)) return;
        setConnection('retrying'); failures++;
        timer = setTimeout(() => void poll(), Math.min(5000, 500 * 2 ** Math.min(failures, 3)));
      }
    }
    void poll(); return () => {stopped = true; abort.abort(); if (timer) clearTimeout(timer);};
  }, [api, sessionID, props.project, props.task, props.active, !!props.writer, control]);
  async function acknowledgeInputIssue() {
    try {await readLatest(); if (current()) {inputLatch.current = false; setInputIssue(undefined); setError(undefined);}}
    catch (problem) {failed(problem);}
  }
  return {connection, error, gap, inputIssue, lastInput, acceptsInput, input, control, exclusive, acknowledgeInputIssue};
}
