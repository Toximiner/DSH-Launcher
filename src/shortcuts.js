'use strict';
const { tr } = require('./i18n');
const { esc } = require('./util');

// Keep the reference and the launcher key dispatcher on the same definitions.
const shortcuts = [
  { keys: 'F5 / Ctrl+R', action: 'reload', codes: ['KeyR'], key: 'F5', shift: false, ru: 'Перезагрузить', en: 'Reload' },
  { keys: 'Shift+F5 / Ctrl+Shift+R', action: 'reloadHard', codes: ['KeyR'], key: 'F5', shift: true, ru: 'Перезагрузить без кэша', en: 'Reload ignoring cache' },
  { keys: 'Ctrl+= / Ctrl+Num+', action: 'zoomIn', codes: ['Equal', 'NumpadAdd'], ru: 'Увеличить масштаб', en: 'Zoom in' },
  { keys: 'Ctrl+- / Ctrl+Num−', action: 'zoomOut', codes: ['Minus', 'NumpadSubtract'], ru: 'Уменьшить масштаб', en: 'Zoom out' },
  { keys: 'Ctrl+0', action: 'zoomReset', codes: ['Digit0', 'Numpad0'], ru: 'Обычный масштаб', en: 'Actual size' },
  { keys: 'Ctrl+F', action: 'find', codes: ['KeyF'], shift: false, ru: 'Найти на странице', en: 'Find in page' },
  { keys: 'F3', action: 'findNext', key: 'F3', shift: false, ru: 'Следующее совпадение', en: 'Next match' },
  { keys: 'Shift+F3', action: 'findPrev', key: 'F3', shift: true, ru: 'Предыдущее совпадение', en: 'Previous match' },
  { keys: 'F11', action: 'fullscreen', key: 'F11', ru: 'Полноэкранный режим', en: 'Full screen' },
  { keys: 'Ctrl+,', action: 'settings', codes: ['Comma'], ru: 'Настройки лаунчера', en: 'Launcher settings' },
];

function keyAction(input) {
  const found = shortcuts.find((s) =>
    (s.shift === undefined || Boolean(input.shift) === s.shift) &&
    ((s.key && input.key === s.key) || (input.control && s.codes?.includes(input.code))));
  return found?.action || null;
}

function shortcutReference() {
  const rows = shortcuts.map((s) => [s.keys, tr(s.ru, s.en), tr('Основное окно', 'Main window')]);
  rows.push(
    ['Ctrl+X / Ctrl+C / Ctrl+V', tr('Вырезать / копировать / вставить', 'Cut / copy / paste'), tr('Поля ввода и выделение', 'Text inputs and selection')],
    ['Ctrl+A', tr('Выделить всё', 'Select all'), tr('Поле ввода или страница', 'Text input or page')],
    ['↑ / ↓', tr('История отправленных сообщений', 'Sent message history'), tr('Поле сообщения; включается в общих настройках', 'Message input; enabled in General settings')],
    ['Esc', tr('Закрыть окно настроек или поиск', 'Close settings or search'), tr('Диалог лаунчера или поиск на странице', 'Launcher dialog or find in page')],
  );
  return `<p>${tr('Клавиши лаунчера. Другие сочетания внутри чата определяет текущая версия dsh.', 'Launcher shortcuts. Other shortcuts inside the chat are defined by the current dsh version.')}</p>
    <input type="search" placeholder="${tr('Поиск по клавише или действию', 'Search by key or action')}" aria-label="${tr('Поиск горячих клавиш', 'Search shortcuts')}" oninput="filterShortcuts(this.value)">
    <table><thead><tr>${[tr('Клавиши', 'Keys'), tr('Действие', 'Action'), tr('Контекст', 'Context')].map((x) => `<th>${esc(x)}</th>`).join('')}</tr></thead><tbody>
    ${rows.map((r) => `<tr data-shortcut>${r.map((x) => `<td>${esc(x)}</td>`).join('')}</tr>`).join('')}</tbody></table>`;
}

module.exports = { shortcuts, keyAction, shortcutReference };
