// Guards against an agent printing a secret by accident. It reads the command the way a shell splits it, so an
// alias, a computed command or a direct socket client passes; the Hisar app's reveal approval is the enforcement.

export type BashVerdict = { block: false } | { block: true; reason: string };

const runForm = "NAME=hisar://<vault>/<item>/<field> hisar run -- <command>";

const revealingSubcommands: Record<string, string> = {
  read: "`hisar read` prints a secret into the transcript.",
  get: "`hisar get` prints an item's secrets into the transcript.",
  export: "`hisar export` writes every secret out in a form the agent can read.",
};

const shells: Record<string, true> = { sh: true, bash: true, zsh: true, dash: true, ksh: true, fish: true };
const wrappers: Record<string, true> = {
  env: true,
  command: true,
  exec: true,
  nohup: true,
  time: true,
  nice: true,
  sudo: true,
  xargs: true,
};
const assignment = /^[A-Za-z_][A-Za-z0-9_]*=/;

interface Parsed {
  commands: string[][];
  substitutions: string[];
}

function readUntilClosingParen(source: string, start: number): number {
  let depth = 1;
  let quote: string | undefined;
  for (let index = start; index < source.length; index++) {
    const character = source[index];
    if (quote) {
      if (character === "\\" && quote === '"') index++;
      else if (character === quote) quote = undefined;
    } else if (character === "\\") index++;
    else if (character === "'" || character === '"') quote = character;
    else if (character === "(") depth++;
    else if (character === ")" && --depth === 0) return index;
  }
  return source.length;
}

function readUntilBacktick(source: string, start: number): number {
  for (let index = start; index < source.length; index++) {
    if (source[index] === "\\") index++;
    else if (source[index] === "`") return index;
  }
  return source.length;
}

// parse splits source into simple commands of unquoted words, and collects the text of $(…) and `…` substitutions.
function parse(source: string): Parsed {
  const parsed: Parsed = { commands: [], substitutions: [] };
  let words: string[] = [];
  let word: string | undefined;
  const endWord = () => {
    if (word !== undefined) words.push(word);
    word = undefined;
  };
  const endCommand = () => {
    endWord();
    if (words.length > 0) parsed.commands.push(words);
    words = [];
  };
  const readSubstitution = (index: number): number => {
    if (source[index] === "`") {
      const end = readUntilBacktick(source, index + 1);
      parsed.substitutions.push(source.slice(index + 1, end));
      return end;
    }
    const end = readUntilClosingParen(source, index + 2);
    parsed.substitutions.push(source.slice(index + 2, end));
    return end;
  };

  for (let index = 0; index < source.length; index++) {
    const character = source[index];
    if (character === " " || character === "\t") {
      endWord();
    } else if ("\n;&|()".includes(character)) {
      endCommand();
    } else if (character === "#" && word === undefined) {
      while (index < source.length && source[index] !== "\n") index++;
      endCommand();
    } else if (character === "\\") {
      word = (word ?? "") + (source[++index] ?? "");
    } else if (character === "'") {
      const end = source.indexOf("'", index + 1);
      const close = end === -1 ? source.length : end;
      word = (word ?? "") + source.slice(index + 1, close);
      index = close;
    } else if (character === '"') {
      word ??= "";
      for (index++; index < source.length && source[index] !== '"'; index++) {
        if (source[index] === "\\") {
          word += source[++index] ?? "";
        } else if (source[index] === "`" || source.startsWith("$(", index)) {
          index = readSubstitution(index);
        } else {
          word += source[index];
        }
      }
    } else if (character === "`" || source.startsWith("$(", index)) {
      word ??= "";
      index = readSubstitution(index);
    } else {
      word = (word ?? "") + character;
    }
  }
  endCommand();
  return parsed;
}

function dropPrefixes(words: string[]): string[] {
  let index = 0;
  while (index < words.length) {
    const word = words[index];
    if (assignment.test(word)) {
      index++;
    } else if (wrappers[word.slice(word.lastIndexOf("/") + 1)] === true) {
      index++;
      while (index < words.length && (words[index].startsWith("-") || assignment.test(words[index]))) index++;
    } else {
      break;
    }
  }
  return words.slice(index);
}

function findShellScript(words: string[]): string | undefined {
  for (let index = 1; index < words.length; index++) {
    const word = words[index];
    if (!word.startsWith("-")) return undefined;
    if (/^-[a-z]*c[a-z]*$/.test(word)) return words[index + 1];
  }
  return undefined;
}

// listHisarInvocations returns the arguments of every hisar command in source, after `hisar`.
export function listHisarInvocations(source: string): string[][] {
  const { commands, substitutions } = parse(source);
  const invocations = substitutions.flatMap(listHisarInvocations);
  for (const command of commands) {
    const words = dropPrefixes(command);
    if (words.length === 0) continue;
    const program = words[0].slice(words[0].lastIndexOf("/") + 1);
    if (program === "hisar") {
      invocations.push(words.slice(1));
    } else if (program === "eval") {
      invocations.push(...listHisarInvocations(words.slice(1).join(" ")));
    } else if (shells[program] === true) {
      const script = findShellScript(words);
      if (script !== undefined) invocations.push(...listHisarInvocations(script));
    }
  }
  return invocations;
}

function hasNoMasking(runArguments: string[]): boolean {
  for (let index = 0; index < runArguments.length; index++) {
    const word = runArguments[index];
    if (word === "--" || !word.startsWith("-")) return false;
    if (/^--?no-masking(=(1|t|T|true|TRUE|True))?$/.test(word)) return true;
    if (word === "--env-file" || word === "-env-file") index++;
  }
  return false;
}

function classifyInvocation(hisarArguments: string[]): BashVerdict {
  let start = 0;
  while (hisarArguments[start] === "--standalone" || hisarArguments[start] === "-standalone") start++;
  const words = hisarArguments.slice(start);
  const subcommand = words[0] ?? "";
  if (Object.hasOwn(revealingSubcommands, subcommand)) {
    const reason = `${revealingSubcommands[subcommand]} Give the secret to the command that needs it instead: ${runForm}`;
    return { block: true, reason };
  }
  if (subcommand === "run" && hasNoMasking(words.slice(1))) {
    return {
      block: true,
      reason: `\`--no-masking\` lets the command print secrets in clear. Run it without that flag, so Hisar masks them: ${runForm}`,
    };
  }
  return { block: false };
}

export function classifyBashCommand(command: string): BashVerdict {
  for (const invocation of listHisarInvocations(command)) {
    const verdict = classifyInvocation(invocation);
    if (verdict.block) return verdict;
  }
  return { block: false };
}
