import { describe, it, expect, afterEach } from 'vitest';
import { render } from 'lit';
import { markdownToTemplate } from '../src/lib/markdown.js';
import '../src/components/atoms/fb-markdown.js';
import type { FbMarkdown } from '../src/components/atoms/fb-markdown.js';

/** Render a TemplateResult into a detached container and return its innerHTML/text. */
function renderToHost(src: string): HTMLElement {
  const host = document.createElement('div');
  render(markdownToTemplate(src), host);
  return host;
}

async function mountAtom(source: string): Promise<FbMarkdown> {
  const el = document.createElement('fb-markdown') as FbMarkdown;
  el.source = source;
  document.body.appendChild(el);
  await el.updateComplete;
  return el;
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('markdownToTemplate — formatting', () => {
  it('renders **bold** as <strong> with no literal asterisks', () => {
    const host = renderToHost('This is **bold** text');
    const strong = host.querySelector('strong');
    expect(strong).toBeTruthy();
    expect(strong!.textContent).toBe('bold');
    expect(host.textContent).not.toContain('*');
  });

  it('renders *italic* and _italic_ as <em>', () => {
    const a = renderToHost('an *emphasised* word');
    const b = renderToHost('an _emphasised_ word');
    expect(a.querySelector('em')!.textContent).toBe('emphasised');
    expect(b.querySelector('em')!.textContent).toBe('emphasised');
    expect(a.textContent).not.toContain('*');
    expect(b.textContent).not.toContain('_');
  });

  it('renders inline `code` as <code>', () => {
    const host = renderToHost('run `make test` now');
    const code = host.querySelector('code');
    expect(code).toBeTruthy();
    expect(code!.textContent).toBe('make test');
    expect(host.textContent).not.toContain('`');
  });

  it('renders an unordered list with one <li> per item', () => {
    const host = renderToHost('- alpha\n- beta\n- gamma');
    const ul = host.querySelector('ul');
    expect(ul).toBeTruthy();
    const items = ul!.querySelectorAll('li');
    expect(items.length).toBe(3);
    expect(items[0]!.textContent).toBe('alpha');
    expect(items[2]!.textContent).toBe('gamma');
    expect(host.querySelector('ol')).toBeNull();
  });

  it('renders an ordered list as <ol>', () => {
    const host = renderToHost('1. first\n2. second');
    const ol = host.querySelector('ol');
    expect(ol).toBeTruthy();
    expect(ol!.querySelectorAll('li').length).toBe(2);
  });

  it('clamps headings to <h3>/<h4> (never <h1>/<h2>)', () => {
    const host = renderToHost('# Title\n\n## Subtitle\n\n### Deeper');
    expect(host.querySelector('h1')).toBeNull();
    expect(host.querySelector('h2')).toBeNull();
    expect(host.querySelector('h3')).toBeTruthy();
    expect(host.querySelector('h4')).toBeTruthy();
    expect(host.querySelector('h3')!.textContent).toContain('Title');
  });

  it('renders a fenced code block as <pre><code> with markers stripped', () => {
    const host = renderToHost('```\nline1\nline2\n```');
    const pre = host.querySelector('pre');
    expect(pre).toBeTruthy();
    expect(pre!.querySelector('code')!.textContent).toBe('line1\nline2');
    expect(host.textContent).not.toContain('```');
  });

  it('splits blank-line-separated paragraphs (parity with old renderReply)', () => {
    const host = renderToHost('First paragraph.\n\nSecond paragraph.');
    const paras = host.querySelectorAll('p');
    expect(paras.length).toBe(2);
    expect(paras[0]!.textContent).toBe('First paragraph.');
    expect(paras[1]!.textContent).toBe('Second paragraph.');
  });

  it('renders plain prose as a single paragraph with no stray markup', () => {
    const host = renderToHost('Just a normal sentence with no formatting.');
    expect(host.querySelectorAll('p').length).toBe(1);
    expect(host.querySelector('strong')).toBeNull();
    expect(host.querySelector('em')).toBeNull();
    expect(host.textContent).toBe('Just a normal sentence with no formatting.');
  });
});

describe('markdownToTemplate — XSS safety', () => {
  it('escapes a raw <img onerror> payload to inert text (no live element)', () => {
    const host = renderToHost('<img src=x onerror=alert(1)>');
    expect(host.querySelector('img')).toBeNull();
    // The angle-bracketed source survives as literal, escaped text.
    expect(host.textContent).toContain('<img src=x onerror=alert(1)>');
  });

  it('escapes a <script> tag to text — never a live script node', () => {
    const host = renderToHost('hello <script>alert(1)</script> world');
    expect(host.querySelector('script')).toBeNull();
    expect(host.textContent).toContain('<script>alert(1)</script>');
  });

  it('does NOT produce an <a> for a [label](javascript:…) link (links unsupported)', () => {
    const host = renderToHost('click [here](javascript:alert(1))');
    expect(host.querySelector('a')).toBeNull();
    // The bracket syntax renders verbatim as escaped text.
    expect(host.textContent).toContain('[here](javascript:alert(1))');
  });

  it('escapes HTML even inside emphasis spans', () => {
    const host = renderToHost('**<b>x</b>**');
    const strong = host.querySelector('strong');
    expect(strong).toBeTruthy();
    // Inner <b> is escaped text, not a real bold element.
    expect(strong!.querySelector('b')).toBeNull();
    expect(strong!.textContent).toBe('<b>x</b>');
  });
});

describe('fb-markdown atom', () => {
  it('renders formatted markdown inside the prose region', async () => {
    const el = await mountAtom('A **bold** claim\n\n- one\n- two');
    const root = el.shadowRoot!;
    expect(root.querySelector('.prose')).toBeTruthy();
    expect(root.querySelector('strong')!.textContent).toBe('bold');
    expect(root.querySelectorAll('li').length).toBe(2);
    expect(root.textContent).not.toContain('**');
  });

  it('renders nothing for empty/whitespace source', async () => {
    const el = await mountAtom('   ');
    expect(el.shadowRoot!.querySelector('.prose')).toBeNull();
  });

  it('does not inject markup from an XSS payload in source', async () => {
    const el = await mountAtom('<img src=x onerror=alert(1)>');
    const root = el.shadowRoot!;
    expect(root.querySelector('img')).toBeNull();
    expect(root.textContent).toContain('<img src=x onerror=alert(1)>');
  });
});
