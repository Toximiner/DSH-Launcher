'use strict';
// Описания релизов: часть на языке окна, Markdown → HTML.
const { esc } = require('./util');

/* Описание релиза (собирается из CHANGELOG: «## Русский … --- ## English …
   **Full Changelog**: …») → часть на языке окна. Нет разметки языков —
   всё описание; строка «Full Changelog» отбрасывается. */
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

/* Маленький markdown для описаний релизов: заголовки (##, ###), списки «- »
   (с переносом строк внутри пункта), абзацы, **жирный**, `код`, ссылки
   [текст](https://…). Текст из сети: сначала всё экранируется, HTML из
   описания в страницу не попадает; ссылки — только http(s). */
function mdToHtml(md) {
  const inline = (t) => esc(t)
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>')
    .replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, (_m, t2, u) => `<a href="${u}" target="_blank">${t2}</a>`);
  const out = [];
  let list = null;  // пункты текущего списка
  let para = null;  // строки текущего абзаца
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
    if (list && /^\s+\S/.test(raw)) { list[list.length - 1] += ' ' + line.trim(); continue; } // перенос внутри пункта
    if (list) flush();
    para = para || [];
    para.push(line.trim());
  }
  flush();
  return out.join('');
}

/* dsh-релизы (GitHub, deepseek-harness) двуязычные: [中文] | [English], части
   отделяет заголовок <h3 id="en-…">…</h3>. Русской части нет — показываем
   EN-часть (с её заголовком, превращённым в ## — mdToHtml знает только
   markdown, HTML экранируется). Якоря нет — описание целиком. Строка
   «Full Changelog: …» отбрасывается. */
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
