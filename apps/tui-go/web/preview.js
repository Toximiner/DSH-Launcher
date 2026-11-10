'use strict';
(async () => {
  const loading = document.getElementById('loading');
  const host = document.getElementById('terminal');
  try {
    const term = new Terminal({fontSize:16,fontFamily:'"DejaVu Sans Mono", monospace',cursorBlink:false,scrollback:0,theme:{background:'#101318',foreground:'#e6edf3'},allowProposedApi:false});
    const fit = new FitAddon.FitAddon();
    term.loadAddon(fit); term.open(host); fit.fit();
    window.previewTerminal = term;
    const ready = new Promise(resolve => window.addEventListener('dsh-tui-ready',resolve,{once:true}));
    const go = new Go();
    const response = await fetch('tui.wasm.gz?v=market-7');
    if (!response.ok) throw new Error('Не удалось загрузить Go-клиент');
    const bytes = await new Response(response.body.pipeThrough(new DecompressionStream('gzip'))).arrayBuffer();
    const instance = await WebAssembly.instantiate(bytes,go.importObject);
    go.run(instance.instance).catch(error => {loading.hidden=false;loading.textContent=error.message;});
    await ready;
    const snapshotResponse = await fetch('snapshot.json');
    if (!snapshotResponse.ok) throw new Error('Не удалось загрузить демонстрационный снимок');
    const snapshot = await snapshotResponse.text();
    dshTui.snapshot(snapshot);
    const marketResponse=await fetch('market-demo.json');
    if (!marketResponse.ok) throw new Error('Не удалось загрузить демо маркета');
    dshTui.marketSnapshot(await marketResponse.text());
    let writing = false;
    let pending = null;
    function render(view) {
      for (const item of document.querySelectorAll('[data-page]')) item.setAttribute('aria-pressed',String(Number(item.dataset.page)===dshTui.currentPage()));
      pending = view;
      if (writing) return;
      function flush() {
        if (pending === null) {writing=false;return;}
        writing=true;const next=pending;pending=null;
        term.write('\x1b[?25l\x1b[H'+next.replace(/\n/g,'\r\n')+'\x1b[0J',flush);
      }
      flush();
    }
    let autoScale = true;
    let requestedFont = 16;
    function resize() {
      let font = requestedFont;
      term.options.fontSize = font;
      if (autoScale) {
        for (let attempt=0;attempt<12;attempt++) {
          const size=fit.proposeDimensions();
          if (!size || (size.cols>=44 && size.rows>=18) || font<=6) break;
          font=Math.max(6,Math.min(font-0.5,Math.floor(font*Math.min(size.cols/44,size.rows/18)*2)/2));
          term.options.fontSize=font;
        }
      }
      fit.fit();
      // Keep a usable virtual terminal even when the user zooms beyond the viewport.
      term.resize(Math.max(44,term.cols),Math.max(18,term.rows));
      render(dshTui.resize(term.cols,term.rows));
    }
    document.getElementById('fit').onclick=()=>{autoScale=true;requestedFont=16;resize();};
    for (const [id,delta] of [['zoom-out',-1],['zoom-in',1]]) {
      document.getElementById(id).onclick=()=>{
        autoScale=false;requestedFont=Math.max(6,Math.min(28,term.options.fontSize+delta));resize();
      };
    }
    for (const button of document.querySelectorAll('[data-page]')) {
      button.onclick=()=>{
        const page=Number(button.dataset.page);
        render(dshTui.page(page));
        if (page===0) term.focus(); else host.querySelector('textarea')?.blur();
      };
    }
    for (const button of document.querySelectorAll('[data-key]')) {
      button.onclick=()=>render(dshTui.key(button.dataset.key));
    }
    function visualResize() {
      if (window.visualViewport) document.documentElement.style.setProperty('--viewport-height',window.visualViewport.height+'px');
      resize();
    }
    window.visualViewport?.addEventListener('resize',visualResize);
    new ResizeObserver(resize).observe(host);
    const keys = {'\r':'enter','\t':'tab','\x1b':'esc','\x7f':'backspace','\x04':'ctrl+d','\x03':'ctrl+c','\x15':'ctrl+u','\x01':'ctrl+a','\x05':'ctrl+e','\x1b[A':'up','\x1b[B':'down','\x1b[C':'right','\x1b[D':'left','\x1b[5~':'pgup','\x1b[6~':'pgdown','\x1b[H':'home','\x1b[F':'end','\x1b[3~':'delete','\x1bOP':'f1','\x1bOS':'f4','\x1b[21~':'f10','\x1b[Z':'shift+tab'};
    term.onData(data => {
      if (keys[data]) {render(dshTui.key(keys[data]));return;}
      if (data.startsWith('\x1b')) return;
      for (const char of data) if (char >= ' ') render(dshTui.key(char));
    });
    host.addEventListener('paste',event => {
      const text=event.clipboardData?.getData('text/plain');
      if (text!==undefined) {event.preventDefault();event.stopImmediatePropagation();render(dshTui.text(text));}
    },true);
    const screen=host.querySelector('.xterm-screen');
    function position(event) {
      const rect=screen.getBoundingClientRect();
      return [Math.floor((event.clientX-rect.left)/rect.width*term.cols),Math.floor((event.clientY-rect.top)/rect.height*term.rows)];
    }
    screen.addEventListener('mousedown',event => {
      if (event.button!==0||event.shiftKey) return;
      event.preventDefault();term.focus();render(dshTui.mouse(...position(event),'click'));
    });
    screen.addEventListener('wheel',event => {
      if (event.shiftKey) return;
      event.preventDefault();render(dshTui.mouse(...position(event),event.deltaY<0?'up':'down'));
    },{passive:false});
    document.getElementById('reset').onclick=()=>location.reload();
    loading.hidden=true;visualResize();
    if (!matchMedia('(pointer: coarse)').matches) term.focus();
    window.dshPreviewReady=true;
  } catch (error) {loading.hidden=false;loading.textContent='Ошибка запуска: '+error.message;}
})();
