import { Trans as LibraryTrans } from "react-i18next";
import type { ComponentProps } from "react";

/** Escape interpolation before the library parses approved translation markup.
 * Unescape only the resulting text nodes: user names containing <strong>,
 * entities or template-looking text must never become component structure. */
export function Trans(props: ComponentProps<typeof LibraryTrans>) {
  return (
    <LibraryTrans
      {...props}
      tOptions={{
        ...props.tOptions,
        interpolation: { ...props.tOptions?.interpolation, escapeValue: true },
      }}
      shouldUnescape
    />
  );
}
