import { Action, ActionPanel, Form, Icon } from "@raycast/api";
import { useState } from "react";
import type { PasswordOptions } from "./hisar";
import { copyGeneratedPassword, pasteValue } from "./raycast/actions";
import { useHisar } from "./raycast/hooks";

interface FormValues {
  length: string;
  digits: boolean;
  symbols: boolean;
}

export default function GeneratePasswordForm() {
  const hisar = useHisar();
  const [lengthError, setLengthError] = useState<string>();

  // Hisar's generator checks the range; the form only makes sure the length is a number.
  function readOptions(values: FormValues): PasswordOptions | undefined {
    const length = values.length.trim();
    if (!/^\d+$/.test(length)) {
      setLengthError("Enter a whole number");
      return undefined;
    }
    return { length: Number(length), digits: values.digits, symbols: values.symbols };
  }

  async function copy(values: FormValues): Promise<void> {
    const options = readOptions(values);
    if (options) await copyGeneratedPassword(() => hisar.generatePassword(options));
  }

  async function paste(values: FormValues): Promise<void> {
    const options = readOptions(values);
    if (options) await pasteValue(() => hisar.generatePassword(options));
  }

  return (
    <Form
      actions={
        <ActionPanel>
          <Action.SubmitForm title="Generate and Copy" icon={Icon.CopyClipboard} onSubmit={copy} />
          <Action.SubmitForm title="Generate and Paste" icon={Icon.Clipboard} onSubmit={paste} />
        </ActionPanel>
      }
    >
      <Form.TextField
        id="length"
        title="Length"
        defaultValue="24"
        error={lengthError}
        onChange={() => setLengthError(undefined)}
      />
      <Form.Checkbox id="digits" label="Digits" defaultValue={true} />
      <Form.Checkbox id="symbols" label="Symbols" defaultValue={true} />
    </Form>
  );
}
