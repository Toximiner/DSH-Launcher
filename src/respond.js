'use strict';
// Ответы схемы dshlauncher:// — чистый модуль (без electron), прямой require.
// blankResponse: навигация произошла, контента нет (пустая тёмная страница);
// noContentResponse: навигация не происходит — страница остаётся как была.

function blankResponse() {
  return new Response('<!doctype html><html><body style="background:#0f1115;margin:0"></body></html>',
    { headers: { 'content-type': 'text/html' } });
}

function noContentResponse() {
  return new Response(null, { status: 204 });
}

module.exports = { blankResponse, noContentResponse };
