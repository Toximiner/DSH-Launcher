'use strict';

/*
 * Unit tests for version comparison (src/versions.js): update checks
 * for the launcher and dsh.
 *
 * Run: node test/version.js   (npm test)
 */

const assert = require('assert');
const { compareDebianVer, compareSemVer, tagToPackageVersion, updateStatus } = require('../src/versions');

// Debian (launcher)
assert.strictEqual(compareDebianVer('1.1.1-1', '1.1.1-1'), 0, 'debian: equal');
assert.ok(compareDebianVer('1.1.2-1', '1.1.1-1') > 0, 'debian: higher upstream');
assert.ok(compareDebianVer('1.1.1-1', '1.1.2-1') < 0, 'debian: lower upstream');
assert.ok(compareDebianVer('1.1.1-2', '1.1.1-1') > 0, 'debian: higher revision');
assert.ok(compareDebianVer('1.1.1', '1.1.1-1') < 0, 'debian: no revision < with revision');
assert.ok(compareDebianVer('2.0.0-1', '1.9.9-9') > 0, 'debian: major');
assert.strictEqual(compareDebianVer('weird', '1.0.0'), 0, 'debian: not recognized → 0');

// semver (dsh)
assert.strictEqual(compareSemVer('0.2.0-rc.2', '0.2.0-rc.2'), 0, 'semver: equal');
assert.ok(compareSemVer('0.2.0', '0.2.0-rc.2') > 0, 'semver: release > rc');
assert.ok(compareSemVer('0.2.0-rc.2', '0.2.0') < 0, 'semver: rc < release');
// prerelease ordering per semver: numbers compare as numbers, shorter comes first, numbers before words
assert.ok(compareSemVer('0.1.7-rc.2', '0.1.7-rc.10') < 0, 'semver: rc.2 < rc.10 (not string-wise)');
assert.ok(compareSemVer('0.1.7-rc.10', '0.1.7-rc.2') > 0, 'semver: rc.10 > rc.2');
assert.ok(compareSemVer('1.0.0-rc', '1.0.0-rc.1') < 0, 'semver: shorter — earlier');
assert.ok(compareSemVer('1.0.0-rc.1', '1.0.0-rc') > 0, 'semver: longer — later');
assert.ok(compareSemVer('1.0.0-alpha', '1.0.0-beta') < 0, 'semver: alpha < beta');
assert.ok(compareSemVer('1.0.0-beta', '1.0.0-alpha') > 0, 'semver: beta > alpha');
assert.ok(compareSemVer('1.0.0-1', '1.0.0-alpha') < 0, 'semver: number before word');
assert.ok(compareSemVer('1.0.0-alpha', '1.0.0-1') > 0, 'semver: word after number');
assert.ok(compareSemVer('1.0.0-alpha.1', '1.0.0-alpha.beta') < 0, 'semver: alpha.1 < alpha.beta');
assert.strictEqual(compareSemVer('x', '1.0.0'), 0, 'semver: not recognized → 0');
assert.ok(compareSemVer('0.2.0-rc.2', '0.2.0') < 0, 'semver: rc < release');
assert.ok(compareSemVer('0.2.1-rc.1', '0.2.0') > 0, 'semver: rc of the next upstream is higher');
assert.ok(compareSemVer('0.2.0-rc.10', '0.2.0-rc.2') > 0, 'semver: rc.10 > rc.2 (number, not string)');
assert.ok(compareSemVer('0.2.0-alpha', '0.2.0-1') > 0, 'semver: alphabetic segment > numeric segment');
assert.ok(compareSemVer('1.0.0-rc.1', '1.0.0-beta.2') > 0, 'semver: rc > beta');
assert.strictEqual(compareSemVer('0.2.0-rc.2', 'garbage'), 0, 'semver: not recognized → 0');

// GitHub tag → package version (CI rule)
assert.strictEqual(tagToPackageVersion('v1.1.1'), '1.1.1-1', 'tag: vX.Y.Z → X.Y.Z-1');
assert.strictEqual(tagToPackageVersion('v1.1.1-R'), '1.1.1-R', 'tag: vX.Y.Z-R → X.Y.Z-R');
assert.strictEqual(tagToPackageVersion('2.0.0'), '2.0.0-1', 'tag: without v');

// update-check result (the «Check for updates» menu)
assert.strictEqual(updateStatus(null, '1.2.0', 'semver'), 'nocheck', 'status: no server response');
assert.strictEqual(updateStatus(null, null, 'deb'), 'nocheck', 'status: no response takes precedence over an unknown version');
assert.strictEqual(updateStatus('1.2.0', null, 'semver'), 'notinstalled', 'status: version unknown');
// launcher package: tag 1.2.0 = package 1.2.0-1
assert.strictEqual(updateStatus('1.2.0', '1.2.0-1', 'deb'), 'uptodate', 'status deb: same version');
assert.strictEqual(updateStatus('1.2.0', '1.2.0-2', 'deb'), 'uptodate', 'status deb: package revision higher than the tag');
assert.strictEqual(updateStatus('1.2.1', '1.2.0-1', 'deb'), 'available', 'status deb: newer');
assert.strictEqual(updateStatus('1.2.0-2', '1.2.0-1', 'deb'), 'available', 'status deb: tag with a newer revision');
// source build: package.json without revision (bug: 1.2.0 showed “1.2.0 available”)
assert.strictEqual(updateStatus('1.2.0', '1.2.0', 'semver'), 'uptodate', 'status src: same version');
assert.strictEqual(updateStatus('1.2.1', '1.2.0', 'semver'), 'available', 'status src: newer');
assert.strictEqual(updateStatus('1.1.0', '1.2.0', 'semver'), 'uptodate', 'status src: local is newer than the release');
// dsh (npm)
assert.strictEqual(updateStatus('0.2.0', '0.2.0-rc.2', 'semver'), 'available', 'status npm: release after an rc');
assert.strictEqual(updateStatus('0.2.0-rc.2', '0.2.0', 'semver'), 'uptodate', 'status npm: rc is not offered over a release');

console.log('OK: version comparison — all checks passed');
