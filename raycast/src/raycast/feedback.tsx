import { Action, ActionPanel, Icon, Keyboard, List, showToast, Toast } from "@raycast/api";
import { openHisar, toHisarError } from "../hisar";
import { describeHisarError, type HisarAction } from "./errorDescription";

const titleByAction: Record<HisarAction, string> = { "open-hisar": "Open Hisar", "update-hisar": "Update Hisar" };

/** showHisarError shows a failure toast, with Open Hisar or Update Hisar when opening Hisar helps. */
export async function showHisarError(error: unknown): Promise<void> {
  const { title, message, action } = describeHisarError(toHisarError(error));
  await showToast({
    style: Toast.Style.Failure,
    title,
    message,
    primaryAction: action && {
      title: titleByAction[action],
      onAction: (toast) => {
        void toast.hide();
        void openHisar().catch(showHisarError);
      },
    },
  });
}

/** ErrorView takes the place of a list Hisar could not load. */
export function ErrorView({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  const hisarError = toHisarError(error);
  const { title, message, action } = describeHisarError(hisarError);
  return (
    <List.EmptyView
      icon={Icon.Warning}
      title={title}
      description={message}
      actions={
        <ActionPanel>
          <Action
            title="Retry"
            icon={Icon.ArrowClockwise}
            shortcut={Keyboard.Shortcut.Common.Refresh}
            onAction={onRetry}
          />
          <Action.CopyToClipboard
            title="Copy Error Details"
            content={`${title}\n${hisarError.code}: ${hisarError.message}`}
          />
          {action && (
            <Action
              title={titleByAction[action]}
              icon={Icon.AppWindow}
              onAction={() => void openHisar().catch(showHisarError)}
            />
          )}
        </ActionPanel>
      }
    />
  );
}
