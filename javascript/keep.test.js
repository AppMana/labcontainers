"use strict";

const {test} = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const {Client} = require("./index");

test("close preserves kept state and releases the daemon to enforce its lease", async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "labcontainers-keep-test-"));
  try {
    const state = path.join(root, "state");
    const kept = path.join(state, "sessions", "kept");
    fs.mkdirSync(kept, {recursive: true});
    let request;
    const signals = [];
    let unref = false;
    const rpc = {
      destroySession(value, options, callback) { request = value; callback(null, {}); },
      close() {},
    };
    const daemon = {kill(signal) { signals.push(signal); }, unref() { unref = true; }};
    const client = new Client("unused", rpc, daemon, root);
    client.stateDirectory = state;
    client.sessions.set("kept", "token");
    await client.close();
    assert.equal(request.preserveKept, true);
    assert.deepEqual(signals, ["SIGINT"]);
    assert.equal(unref, true);
    assert.equal(fs.existsSync(kept), true);
  } finally {
    fs.rmSync(root, {recursive: true, force: true});
  }
});
