/** The launch context of a view command that brought itself back after Hisar's approval. */
export const afterApprovalContext = { isAfterApproval: true };

export interface LoadAttempt {
  /** wasApprovalNeeded: launcher.hello said Raycast was not approved yet, so Hisar's approval window took focus. */
  wasApprovalNeeded: boolean;
  isLoaded: boolean;
  launchContext?: Record<string, unknown>;
}

/**
 * isReopeningAfterApproval: Hisar's approval window hides Raycast, so a view command that loaded after an approval
 * launches itself again to show what it loaded. Only once: a launch that is that reopening never reopens.
 */
export function isReopeningAfterApproval(attempt: LoadAttempt): boolean {
  return attempt.wasApprovalNeeded && attempt.isLoaded && attempt.launchContext?.isAfterApproval !== true;
}
