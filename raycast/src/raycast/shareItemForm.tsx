import { Action, ActionPanel, Form, Icon, useNavigation } from "@raycast/api";
import { useState } from "react";
import type { Hisar, Item, ShareableField } from "../hisar";
import { shareItem } from "./actions";
import { listShareableFields } from "./itemActions";

const labelByField: Record<ShareableField, string> = { username: "Username", password: "Password" };

/** The lifetimes Hisar's share links allow (apitypes.IsAllowedShareLinkExpiry). */
const allExpiry = [
  { title: "1 hour", seconds: 3_600 },
  { title: "1 day", seconds: 86_400 },
  { title: "7 days", seconds: 604_800 },
  { title: "30 days", seconds: 2_592_000 },
];

/** 0 is unlimited, as on Hisar's share links. */
const allViewLimit = [
  { title: "1 view", views: 1 },
  { title: "5 views", views: 5 },
  { title: "10 views", views: 10 },
  { title: "Unlimited", views: 0 },
];

type ShareFormValues = Partial<Record<ShareableField, boolean>> & { expiresInSeconds: string; maxViews: string };

/** ShareItemForm picks what a share link shows and for how long; Hisar asks before it creates the link. */
export function ShareItemForm({ hisar, item }: { hisar: Hisar; item: Item }) {
  const { pop } = useNavigation();
  const allField = listShareableFields(item);
  const [fieldError, setFieldError] = useState<string>();

  async function share(values: ShareFormValues): Promise<void> {
    const allChosen = allField.filter((field) => values[field]);
    if (allChosen.length === 0) {
      setFieldError("Choose at least one field");
      return;
    }
    const options = {
      allField: allChosen,
      expiresInSeconds: Number(values.expiresInSeconds),
      maxViews: Number(values.maxViews),
    };
    if (await shareItem(hisar, item, options)) pop();
  }

  return (
    <Form
      navigationTitle={`Share ${item.title}`}
      actions={
        <ActionPanel>
          <Action.SubmitForm title="Create and Copy Share Link" icon={Icon.Link} onSubmit={share} />
        </ActionPanel>
      }
    >
      <Form.Description text="Anyone with the link sees these fields until it expires or runs out of views." />
      {allField.map((field, index) => (
        <Form.Checkbox
          key={field}
          id={field}
          title={index === 0 ? "Fields" : undefined}
          label={labelByField[field]}
          defaultValue={true}
          error={index === 0 ? fieldError : undefined}
          onChange={() => setFieldError(undefined)}
        />
      ))}
      <Form.Dropdown id="expiresInSeconds" title="Expires After" defaultValue={String(604_800)}>
        {allExpiry.map((expiry) => (
          <Form.Dropdown.Item key={expiry.seconds} title={expiry.title} value={String(expiry.seconds)} />
        ))}
      </Form.Dropdown>
      <Form.Dropdown id="maxViews" title="Views" defaultValue="1">
        {allViewLimit.map((limit) => (
          <Form.Dropdown.Item key={limit.views} title={limit.title} value={String(limit.views)} />
        ))}
      </Form.Dropdown>
    </Form>
  );
}
