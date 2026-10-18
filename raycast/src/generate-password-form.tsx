import { Action, ActionPanel, Form, Icon, Keyboard } from "@raycast/api";
import { usePromise } from "@raycast/utils";
import { useState } from "react";
import { toHisarError } from "./hisar";
import { copyGeneratedPassword, pasteValue } from "./raycast/actions";
import { showHisarError } from "./raycast/feedback";
import { useHisar } from "./raycast/hooks";

/** GeneratePasswordForm shows a password from Hisar's generator and makes a new one whenever an option changes. */
export default function GeneratePasswordForm() {
  const hisar = useHisar();
  const [length, setLength] = useState("24");
  const [digits, setDigits] = useState(true);
  const [symbols, setSymbols] = useState(true);
  const isWholeNumber = /^\d+$/.test(length.trim());
  const {
    data: password,
    isLoading,
    error,
    revalidate,
  } = usePromise(
    (length: number, digits: boolean, symbols: boolean) => hisar.generatePassword({ length, digits, symbols }),
    [Number(length.trim()), digits, symbols],
    {
      execute: isWholeNumber,
      onError: (error) => {
        if (toHisarError(error).code !== "invalid-request") void showHisarError(error);
      },
    },
  );
  // Hisar's generator checks the range and says which lengths it accepts: "choose a length between 4 and 256".
  const rangeError =
    error && toHisarError(error).code === "invalid-request"
      ? error.message.charAt(0).toUpperCase() + error.message.slice(1)
      : undefined;

  return (
    <Form
      isLoading={isLoading}
      actions={
        <ActionPanel>
          {password && (
            <Action
              title="Copy Password"
              icon={Icon.CopyClipboard}
              shortcut={Keyboard.Shortcut.Common.Copy}
              onAction={() => copyGeneratedPassword(async () => password)}
            />
          )}
          {password && (
            <Action title="Paste Password" icon={Icon.Clipboard} onAction={() => pasteValue(async () => password)} />
          )}
          <Action
            title="Regenerate Password"
            icon={Icon.ArrowClockwise}
            shortcut={Keyboard.Shortcut.Common.Refresh}
            onAction={revalidate}
          />
        </ActionPanel>
      }
    >
      <Form.Description title="Password" text={password ?? ""} />
      <Form.TextField
        id="length"
        title="Length"
        value={length}
        error={isWholeNumber ? rangeError : "Enter a whole number"}
        onChange={setLength}
      />
      <Form.Checkbox id="digits" label="Digits" value={digits} onChange={setDigits} />
      <Form.Checkbox id="symbols" label="Symbols" value={symbols} onChange={setSymbols} />
    </Form>
  );
}
