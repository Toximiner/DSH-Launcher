'use strict';

// npm does not expose a total download size. Show completed fetches and an
// indeterminate activity indicator instead of inventing an overall percentage.
class InstallProgress {
  constructor(started = Date.now()) {
    this.started = started;
    this.phase = 'Preparing dependencies';
    this.requests = new Set();
    this.packages = new Set();
  }

  observe(line) {
    const match = /\bnpm http (fetch|cache)\b.*?(https?:\/\/\S+)/.exec(line);
    if (!match) return;
    if (match[1] === 'fetch' && !/\bnpm http fetch \S+ [23]\d\d\s/.test(line)) return;
    this.requests.add(match[2]);
    if (/\.tgz(?:\?|$)/.test(match[2])) this.packages.add(match[2]);
    this.phase = this.packages.size ? 'Fetching packages' : 'Resolving packages';
  }

  text(now = Date.now()) {
    const elapsed = Math.max(0, now - this.started);
    const spinner = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'][Math.floor(elapsed / 100) % 10];
    const count = this.packages.size ? `${this.packages.size} packages fetched` : `${this.requests.size} requests completed`;
    return `${spinner} ${this.phase} · ${count} · ${Math.floor(elapsed / 1000)}s · /cancel`;
  }
}

module.exports = { InstallProgress };
