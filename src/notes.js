'use strict';
// Release descriptions: the part in the window's language, Markdown → HTML.
const { esc } = require('./util');

/* Release description (assembled from CHANGELOG: "## Русский … --- ## English …
   **Full Changelog**: …") → the part in the window's language. No language markup —
   the whole description; the "Full Changelog" line is dropped. */
function releaseNotesLang(body, lang) {
  const text = String(body || '').replace(/\r\n/g, '\n');
  const ru = text.match(/^##\s*Русский\s*$([\s\S]*?)(?=^---\s*$|^##\s*English\s*$|$(?![\s\S]))/m);
  const en = text.match(/^##\s*English\s*$([\s\S]*)/m);
  let part = text;
  if (lang === 'ru' && ru) part = ru[1];
  else if (lang !== 'ru' && en) part = en[1];
  else if (ru) part = ru[1];
  return part.replace(/^\*\*Full Changelog\*\*.*$/gm, '').replace(/^---\s*$/gm, '').trim();
}

/* A small markdown for release descriptions: headings (##, ###), lists "- "
   (with line breaks inside an item), paragraphs, **bold**, `code`, links
   [text](https://…). The text comes from the network: everything is escaped first, no HTML from
   the description reaches the page; links — only http(s). */
function mdToHtml(md) {
  const inline = (t) => esc(t)
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>')
    .replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, (_m, t2, u) => `<a href="${u}" target="_blank">${t2}</a>`);
  const out = [];
  let list = null;  // the items of the current list
  let para = null;  // the lines of the current paragraph
  const flush = () => {
    if (list) { out.push('<ul>' + list.map((li) => `<li>${inline(li)}</li>`).join('') + '</ul>'); list = null; }
    if (para) { out.push(`<p>${inline(para.join(' '))}</p>`); para = null; }
  };
  for (const raw of String(md || '').split('\n')) {
    const line = raw.replace(/\s+$/, '');
    const h = line.match(/^#{2,4}\s+(.*)$/);
    const li = line.match(/^\s*[-*]\s+(.*)$/);
    if (!line.trim()) { flush(); continue; }
    if (h) { flush(); out.push(`<h4>${inline(h[1])}</h4>`); continue; }
    if (li) { if (para) flush(); list = list || []; list.push(li[1]); continue; }
    if (list && /^\s+\S/.test(raw)) { list[list.length - 1] += ' ' + line.trim(); continue; } // a line break inside an item
    if (list) flush();
    para = para || [];
    para.push(line.trim());
  }
  flush();
  return out.join('');
}

/* dsh releases (GitHub, deepseek-harness) are bilingual: [中文] | [English], the parts
   are separated by the heading <h3 id="en-…">…</h3>. No Russian part — we show the
   EN part (with its heading converted to ## — mdToHtml knows only
   markdown, HTML is escaped). No anchor — the whole description. The line
   "Full Changelog: …" is dropped. */
function dshNotesPart(body) {
  const lines = String(body || '').replace(/\r\n/g, '\n').split('\n');
  const at = lines.findIndex((l) => /^<h3 id="en-[^"]*"/.test(l));
  return (at === -1 ? lines : lines.slice(at))
    .map((l) => l.replace(/^<h3 id="[^"]*">([^<]*)<\/h3>\s*$/, '## $1'))
    .join('\n')
    .replace(/^Full Changelog:.*$/gm, '')
    .replace(/^\*\*Full Changelog\*\*.*$/gm, '')
    .replace(/^---\s*$/gm, '')
    .trim();
}

module.exports = { releaseNotesLang, mdToHtml, dshNotesPart };
