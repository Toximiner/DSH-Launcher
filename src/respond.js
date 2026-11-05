'use strict';
// dshlauncher:// scheme responses — pure module (no electron), direct require.
// blankResponse: navigation happened, no content (empty dark page);
// noContentResponse: no navigation — the page stays as it was.

function blankResponse() {
  return new Response('<!doctype html><html><body style="background:#0f1115;margin:0"></body></html>',
    { headers: { 'content-type': 'text/html' } });
}

function noContentResponse() {
  return new Response(null, { status: 204 });
}

module.exports = { blankResponse, noContentResponse };
