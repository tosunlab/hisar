import { findAllCardScope } from "./card-detection";
import { captureLogin, findAllLoginScope, isSubmitLikeButton, type LoginScope } from "./detection";
import { containsAcrossShadow, isInputElement } from "./dom";

export type Submission = { kind: "login"; username: string; password: string } | { kind: "username"; username: string };

export interface SubmissionCaptureOptions {
  onSubmission: (submission: Submission) => void;
  now?: () => number;
  duplicateWindowMilliseconds?: number;
}

function findScopeContaining(document: Document, element: Element): LoginScope | undefined {
  const allScope = findAllLoginScope(document);
  const allContaining = allScope.filter((scope) => scope.root === element || containsAcrossShadow(scope.root, element));
  return allContaining.find((scope) => scope.kind !== "username-only") ?? allContaining[0];
}

export function readSubmission(scope: LoginScope): Submission | undefined {
  if (scope.kind === "username-only") {
    const username = scope.usernameField?.value.trim() ?? "";
    return username ? { kind: "username", username } : undefined;
  }
  const captured = captureLogin(scope);
  return captured ? { kind: "login", ...captured } : undefined;
}

// isPaymentScope: the "password" is a card's security code, as on a payment form that conceals it, so there is no login to
// save.
function isPaymentScope(document: Document, scope: LoginScope): boolean {
  if (scope.allPasswordField.length === 0) return false;
  const allCardElement = new Set(findAllCardScope(document).flatMap((cardScope) => cardScope.allField.map((field) => field.element)));
  return scope.allPasswordField.some((field) => allCardElement.has(field));
}

function findSubmitLikeTarget(event: Event): Element | undefined {
  for (const target of event.composedPath()) {
    if (!(target instanceof Element)) continue;
    if (isSubmitLikeButton(target)) return target;
    if (target.tagName === "FORM" || target.tagName === "BODY") return undefined;
  }
  return undefined;
}

export function attachSubmissionCapture(document: Document, options: SubmissionCaptureOptions): () => void {
  const now = options.now ?? (() => Date.now());
  const duplicateWindow = options.duplicateWindowMilliseconds ?? 3000;
  let lastSignature = "";
  let lastTime = -Infinity;

  const report = (scope: LoginScope | undefined): void => {
    if (!scope || isPaymentScope(document, scope)) return;
    const submission = readSubmission(scope);
    if (!submission) return;
    const signature = JSON.stringify(submission);
    const time = now();
    if (signature === lastSignature && time - lastTime < duplicateWindow) return;
    lastSignature = signature;
    lastTime = time;
    options.onSubmission(submission);
  };

  const handleSubmit = (event: Event): void => {
    const form = event.composedPath()[0];
    if (form instanceof Element) report(findScopeContaining(document, form));
  };

  const handleKeydown = (event: KeyboardEvent): void => {
    if (event.key !== "Enter" || event.isComposing) return;
    const target = event.composedPath()[0];
    if (target instanceof Element && isInputElement(target)) report(findScopeContaining(document, target));
  };

  const handleClick = (event: MouseEvent): void => {
    const button = findSubmitLikeTarget(event);
    if (button) report(findScopeContaining(document, button));
  };

  document.addEventListener("submit", handleSubmit, true);
  document.addEventListener("keydown", handleKeydown, true);
  document.addEventListener("click", handleClick, true);
  return () => {
    document.removeEventListener("submit", handleSubmit, true);
    document.removeEventListener("keydown", handleKeydown, true);
    document.removeEventListener("click", handleClick, true);
  };
}
