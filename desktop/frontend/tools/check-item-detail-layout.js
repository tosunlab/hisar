// Run in the 900 × 480 preview with GitHub selected: await import('/tools/check-item-detail-layout.js').
const detail = document.querySelector('.detail');
const code = document.querySelector('[data-testid="one-time-code-value"]');
const seconds = document.querySelector('[data-testid="one-time-code-seconds"]');
const strength = document.querySelector('[data-testid="strength-meter"] .label');
if (!detail || !code || !seconds || !strength) throw new Error('Select the GitHub login before running the layout check.');
const originalStyle = detail.getAttribute('style');
function check(condition, message) {
  if (!condition) throw new Error(message);
}
function singleLine(element) {
  const range = document.createRange();
  range.selectNodeContents(element);
  return range.getClientRects().length === 1;
}
try {
  for (const width of [null, 320, 400, 479]) {
    if (width !== null) {
      detail.style.width = `${width}px`;
      detail.style.flex = 'none';
    }
    await new Promise(requestAnimationFrame);
    const value = code.closest('.code-value');
    const label = code.closest('.field').querySelector('.field-label');
    const codeRect = code.getBoundingClientRect();
    const countdownRect = seconds.getBoundingClientRect();
    check(singleLine(code), `Code wraps at ${width ?? 'minimum window'} width`);
    check(singleLine(seconds), `Countdown wraps at ${width ?? 'minimum window'} width`);
    check(singleLine(strength), `Strength wraps at ${width ?? 'minimum window'} width`);
    check(Math.abs(codeRect.y + codeRect.height / 2 - countdownRect.y - countdownRect.height / 2) < 2, 'Code and countdown must share a row');
    check(label.getBoundingClientRect().bottom <= value.getBoundingClientRect().top, 'Label must sit above the value');
    check(value.scrollWidth <= value.clientWidth + 1, `Code and countdown overflow their value track at ${width}`);
    check(strength.getBoundingClientRect().right <= strength.closest('.field-value').getBoundingClientRect().right + 1, 'Strength must fit its value track');
  }
  console.info('Item detail layout passes at the minimum window and 320px, 400px, and 479px detail widths.');
} finally {
  if (originalStyle === null) detail.removeAttribute('style');
  else detail.setAttribute('style', originalStyle);
}
