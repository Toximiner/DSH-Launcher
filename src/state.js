'use strict';
// Общее состояние лаунчера — то, что читают и меняют разные части. Начальные
// значения задаёт main.js (там же, где раньше были объявления let).
//
// Операции с собственным процессом/таймером (установка dsh, установка
// маркета, шаг обновления) имеют разные формы состояния — но при закрытии
// всё одинаково: SIGTERM процессу и clearInterval. Модули регистрируют
// функцию очистки через settleOperation при загрузке, before-quit вызывает
// все через settleAllOperations — без знания списка модулей.
const operationSettlers = [];

module.exports = {
  win: null,                 // главное окно
  stopping: false,           // лаунчер закрывается
  dshBin: null,              // программа dsh (DSH_BIN; найденная может смениться)
  dshProc: null,             // процесс dsh, запущенный лаунчером
  weStartedDsh: false,       // dsh запустил лаунчер (при закрытии — остановить)
  dshFatal: false,           // dsh не запустился / упал — не перезапускать
  adoptedDsh: false,         // dsh «усыновлён» после самовозрождения
  adoptedOldPgid: null,      // группа исходного dsh до самовозрождения
  authUrl: null,             // URL с токеном, напечатанный dsh
  launching: false,          // идёт запуск dsh (вопросы об обновлениях и т. п.)
  restarting: false,         // перезапуск в новую версию после обновления
  relaunchRequested: false,  // «Перезапустить лаунчер» (настройки, «Обновить сейчас»)
  dshInstalledVersion: null, // версия dsh, проверенная при запуске
  currentPage: null,         // перерисовка текущей служебной страницы (смена языка)
  portState: null,           // null | 'up' | 'down' — порт dsh (авто-восстановление)
  dialogView: null,          // диалог — слой в главном окне (WebContentsView)
  dialogRender: null,        // перерисовка содержимого диалога: (copied) => void
  dialogCopyText: '',        // текст «Скопировать для отчёта»
  tray: null,                // значок в трее (Tray) или null
  lastReportPath: null,      // последний сохранённый отчёт («Показать в папке»)
  settingsSection: 'general', // раздел окна «Настройки» (меню слева)
  availableUpdates: {},      // найденные обновления: { launcher, dsh } → релиз (.version …)
  updateChecks: { launcher: null, dsh: null }, // проверки обновлений при запуске (промисы)
  launcherUpdatedOnDisk: false, // пакет лаунчера обновлён, а работает старый процесс
  openContextMenu: null,     // открытое контекстное меню (закрыть по маршруту)
  findView: null,            // панель поиска по странице (Ctrl+F)
  spellcheckOn: true,        // проверка орфографии в полях ввода
  restartingDsh: false,      // идёт «Перезапустить dsh»: остановка dsh — не сбой
  dshCrashNotice: null,      // dsh перезапущен после сбоя — плашка, когда GUI загрузится
  settleOperation(fn) { operationSettlers.push(fn); },
  settleAllOperations() { for (const fn of operationSettlers) fn(); },
};
