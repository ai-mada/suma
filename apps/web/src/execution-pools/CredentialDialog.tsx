import { tr, useLocale, UiError } from "../i18n";
import { useState } from "react";
import { Modal } from "../ui";
import type { Credential } from "./model";
export function CredentialDialog({
  value,
  onClose,
}: {
  value?: Credential;
  onClose: () => void;
}) {
  useLocale();
  const [revealed, setRevealed] = useState(false);
  return (
    <Modal
      open={!!value}
      onClose={onClose}
      title={tr("pools.credential.title")}
      description={tr("pools.credential.secret_notice")}
    >
      {value && (
        <>
          <strong>{value.runner.name}</strong>
          <p>
            <code>{value.runner.worker_id}</code>
          </p>
          {value.token ? (
            <>
              <label className="field">
                <span>{tr("pools.credential.one_time")}</span>
                <input
                  aria-label={tr("pools.credential.one_time")}
                  autoComplete="off"
                  readOnly
                  type={revealed ? "text" : "password"}
                  value={value.token}
                />
              </label>
              <button onClick={() => setRevealed((v) => !v)}>
                {revealed
                  ? tr("pools.credential.hide")
                  : tr("pools.credential.show")}
              </button>
              <p>{tr("pools.credential.save_notice")}</p>
            </>
          ) : (
            <p className="warning">
              {tr("pools.credential.recovered_without_secret")}
            </p>
          )}
          <div className="dialog-actions">
            <button className="primary" onClick={onClose}>
              {value.token
                ? tr("pools.credential.saved_close")
                : tr("pools.credential.close_manage")}
            </button>
          </div>
        </>
      )}
    </Modal>
  );
}
