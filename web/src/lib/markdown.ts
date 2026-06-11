import { html, type TemplateResult } from 'lit';

/**
 * markdown.ts — a tiny, deliberately-minimal, XSS-safe markdown → Lit renderer.
 *
 * TECH_STACK.md forbids a markdown dependency, so this is hand-rolled to cover
 * exactly the subset the native-AI surfaces emit (briefing, assistant chat,
 * advisories): paragraphs, `**bold**`, `*italic*`/`_italic_`, inline `` `code` ``,
 * unordered/ordered lists, fenced/indented code blocks, and clamped headings.
 *
 * SECURITY GUARANTEE (the whole point of building this in-repo):
 *   Every byte of model/user text reaches the DOM ONLY through a `${value}` slot
 *   inside an `html` template literal. Lit escapes interpolated string values,
 *   so any `<`, `>`, `&`, quotes, or `<script>`/`<img onerror=…>` payload renders
 *   as inert TEXT, never as live markup. The ONLY static HTML in the output tree
 *   is the wrapping tags THIS file authors (<p>, <strong>, <em>, <code>, <ul>,
 *   <ol>, <li>, <pre>, <h3>, <h4>). We never call `unsafeHTML`, never parse model
 *   output with `DOMParser`/`innerHTML`, and never emit links/images/raw HTML —
 *   so there is no sink for injected markup.
 */

// ---------------------------------------------------------------------------
// Block tokenizer
// ---------------------------------------------------------------------------

type Block =
  | { kind: 'p'; lines: string[] }
  | { kind: 'ul'; items: string[] }
  | { kind: 'ol'; items: string[] }
  | { kind: 'h'; level: 3 | 4; text: string }
  | { kind: 'code'; text: string };

const UL_RE = /^\s*[-*]\s+(.*)$/;
const OL_RE = /^\s*\d+[.)]\s+(.*)$/;
const HEADING_RE = /^(#{1,6})\s+(.*)$/;
const FENCE_RE = /^\s*```/;

/**
 * Split a markdown source string into a flat list of block tokens. Pure string
 * work — no DOM, no HTML. Inline markup inside each block is left untouched here
 * and handled by {@link inlineToTemplate}.
 */
function tokenize(src: string): Block[] {
  const blocks: Block[] = [];
  // Normalize newlines so blank-line/paragraph logic is platform-independent.
  const lines = src.replace(/\r\n?/g, '\n').split('\n');

  // `noUncheckedIndexedAccess` is on, so array access is `string | undefined`.
  // `at` returns '' past EOF, letting the regex guards below stay total.
  const at = (idx: number): string => lines[idx] ?? '';

  let i = 0;
  while (i < lines.length) {
    const line = at(i);

    // Fenced code block: consume verbatim until the closing fence (or EOF).
    if (FENCE_RE.test(line)) {
      const code: string[] = [];
      i++;
      while (i < lines.length && !FENCE_RE.test(at(i))) {
        code.push(at(i));
        i++;
      }
      if (i < lines.length) i++; // skip closing fence
      blocks.push({ kind: 'code', text: code.join('\n') });
      continue;
    }

    // Blank line: paragraph separator, nothing to emit.
    if (line.trim() === '') {
      i++;
      continue;
    }

    // Heading (clamped to h3/h4 so a page never gains a competing h1/h2).
    const heading = HEADING_RE.exec(line);
    if (heading) {
      const level = (heading[1] ?? '').length <= 1 ? 3 : 4;
      blocks.push({ kind: 'h', level, text: (heading[2] ?? '').trim() });
      i++;
      continue;
    }

    // Unordered list: consume the contiguous run of `- ` / `* ` lines.
    if (UL_RE.test(line)) {
      const items: string[] = [];
      let m: RegExpExecArray | null;
      while (i < lines.length && (m = UL_RE.exec(at(i)))) {
        items.push(m[1] ?? '');
        i++;
      }
      blocks.push({ kind: 'ul', items });
      continue;
    }

    // Ordered list: consume the contiguous run of `1. ` lines.
    if (OL_RE.test(line)) {
      const items: string[] = [];
      let m: RegExpExecArray | null;
      while (i < lines.length && (m = OL_RE.exec(at(i)))) {
        items.push(m[1] ?? '');
        i++;
      }
      blocks.push({ kind: 'ol', items });
      continue;
    }

    // Paragraph: consume until a blank line or a structural line begins.
    const para: string[] = [];
    while (
      i < lines.length &&
      at(i).trim() !== '' &&
      !UL_RE.test(at(i)) &&
      !OL_RE.test(at(i)) &&
      !HEADING_RE.test(at(i)) &&
      !FENCE_RE.test(at(i))
    ) {
      para.push(at(i).trim());
      i++;
    }
    blocks.push({ kind: 'p', lines: para });
  }

  return blocks;
}

// ---------------------------------------------------------------------------
// Inline renderer
// ---------------------------------------------------------------------------

// Ordered so `**bold**` is matched before single-`*` italic. `code` is matched
// first so markers inside a code span are treated as literal text.
const INLINE_RE = /(`[^`]+`|\*\*[^*]+\*\*|\*[^*\n]+\*|_[^_\n]+_)/g;

/**
 * Render a single line/run of text with inline markup. Returns an array of Lit
 * values: bare strings (which Lit escapes) for plain runs, and small `html`
 * fragments for emphasis spans whose INNER text is itself an escaped `${value}`.
 */
function inlineToTemplate(text: string): Array<string | TemplateResult> {
  const out: Array<string | TemplateResult> = [];
  let last = 0;
  for (const match of text.matchAll(INLINE_RE)) {
    const token = match[0];
    const start = match.index ?? 0;
    if (start > last) out.push(text.slice(last, start)); // plain run (escaped)

    if (token.startsWith('`')) {
      out.push(html`<code>${token.slice(1, -1)}</code>`);
    } else if (token.startsWith('**')) {
      out.push(html`<strong>${token.slice(2, -2)}</strong>`);
    } else if (token.startsWith('*')) {
      out.push(html`<em>${token.slice(1, -1)}</em>`);
    } else {
      // _italic_
      out.push(html`<em>${token.slice(1, -1)}</em>`);
    }
    last = start + token.length;
  }
  if (last < text.length) out.push(text.slice(last)); // trailing plain run
  return out;
}

/** Join paragraph lines into one inline stream, preserving intra-paragraph breaks. */
function paragraphInline(lines: string[]): Array<string | TemplateResult> {
  const parts: Array<string | TemplateResult> = [];
  lines.forEach((line, idx) => {
    if (idx > 0) parts.push(html`<br />`);
    parts.push(...inlineToTemplate(line));
  });
  return parts;
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Render markdown `src` into a Lit `TemplateResult` tree. Safe to bind directly
 * in a component template: `html\`${markdownToTemplate(value)}\``. All text is
 * escaped by Lit; only the structural tags authored here are real markup.
 */
export function markdownToTemplate(src: string): TemplateResult {
  const blocks = tokenize(src ?? '');
  return html`${blocks.map((block) => {
    switch (block.kind) {
      case 'code':
        // Code text is an interpolated value → escaped, never executed.
        return html`<pre><code>${block.text}</code></pre>`;
      case 'h':
        return block.level === 3
          ? html`<h3>${inlineToTemplate(block.text)}</h3>`
          : html`<h4>${inlineToTemplate(block.text)}</h4>`;
      case 'ul':
        return html`<ul>
          ${block.items.map((item) => html`<li>${inlineToTemplate(item)}</li>`)}
        </ul>`;
      case 'ol':
        return html`<ol>
          ${block.items.map((item) => html`<li>${inlineToTemplate(item)}</li>`)}
        </ol>`;
      case 'p':
      default:
        return html`<p>${paragraphInline(block.lines)}</p>`;
    }
  })}`;
}
