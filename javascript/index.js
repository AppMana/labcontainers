"use strict";

const grpc = require("@grpc/grpc-js");
const protoLoader = require("@grpc/proto-loader");
const childProcess = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const protoPath = path.join(__dirname, "..", "api", "v1", "labcontainers.proto");
const maxMessageBytes = 257 * 1024 * 1024;
const definition = protoLoader.loadSync(protoPath, {
  keepCase: false,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true,
});
const Service = grpc.loadPackageDefinition(definition).labcontainers.v1.Labcontainers;

function unary(client, method, request, options = {}) {
  return new Promise((resolve, reject) => {
    client[method](request, options, (error, value) => error ? reject(error) : resolve(value));
  });
}

function ready(client, deadlineMs = 10000) {
  return new Promise((resolve, reject) => {
    client.waitForReady(Date.now() + deadlineMs, error => error ? reject(error) : resolve());
  });
}

class Client {
  constructor(socket, rpc, process = null, temporaryDirectory = null) {
    this.socket = socket;
    this.rpc = rpc;
    this.process = process;
    this.temporaryDirectory = temporaryDirectory;
    this.sessions = new Map();
  }

  static async dial(socket) {
    const rpc = new Service(`unix://${socket}`, grpc.credentials.createInsecure(), {
      "grpc.max_send_message_length": maxMessageBytes,
      "grpc.max_receive_message_length": maxMessageBytes,
    });
    await ready(rpc);
    return new Client(socket, rpc);
  }

  static async launch({socket, stateDir, labd = "labd"} = {}) {
    const temporaryDirectory = socket ? null : fs.mkdtempSync(path.join(os.tmpdir(), "labcontainers-"));
    socket ||= path.join(temporaryDirectory, "labd.sock");
    stateDir ||= temporaryDirectory ? path.join(temporaryDirectory, "state") : undefined;
    const args = ["--socket", socket, "--parent-pid", String(process.pid)];
    if (stateDir) args.push("--state-dir", stateDir);
    const daemon = childProcess.spawn(labd, args, {stdio: "inherit"});
    daemon.once("error", () => {});
    try {
      const client = await Client.dial(socket);
      client.process = daemon;
      client.temporaryDirectory = temporaryDirectory;
      client.stateDirectory = stateDir;
      return client;
    } catch (error) {
      daemon.kill("SIGKILL");
      if (temporaryDirectory) fs.rmSync(temporaryDirectory, {recursive: true, force: true});
      throw error;
    }
  }

  async start(spec, {ttlSeconds = 7200, labels = {}} = {}) {
    const value = await unary(this.rpc, "createSession", {spec, ttlSeconds, labels});
    this.sessions.set(value.id, value.resumeToken);
    return new Session(this, value);
  }

  async resume(id) {
    const value = await unary(this.rpc, "getSession", {id});
    this.sessions.set(value.id, value.resumeToken);
    return new Session(this, value);
  }

  async close() {
    let firstError;
    for (const [id, resumeToken] of this.sessions) {
      try {
        await unary(this.rpc, "destroySession", {id, resumeToken, preserveKept: true}, {deadline: Date.now() + 120000});
      } catch (error) {
        if (error.code !== grpc.status.NOT_FOUND) firstError ||= error;
      }
    }
    this.sessions.clear();
    this.rpc.close();
    if (this.process) {
      let retain = true;
      if (this.stateDirectory) {
        try { retain = fs.readdirSync(path.join(this.stateDirectory, "sessions")).length !== 0; }
        catch (_) { /* Uncertain state must not be erased. */ }
      }
      this.process.kill("SIGINT");
      if (retain) {
        this.process.unref();
        if (firstError) throw firstError;
        return;
      }
      await new Promise(resolve => {
        const timer = setTimeout(() => { this.process.kill("SIGKILL"); resolve(); }, 10000);
        this.process.once("exit", () => { clearTimeout(timer); resolve(); });
      });
    }
    if (this.temporaryDirectory) fs.rmSync(this.temporaryDirectory, {recursive: true, force: true});
    if (firstError) throw firstError;
  }
}

class Session {
  constructor(client, value) { this.client = client; this.value = value; }
  get id() { return this.value.id; }
  node(name) { return new Node(this, name); }
  async keep(ttlSeconds = 86400) {
    this.value = await unary(this.client.rpc, "keepSession", {id: this.id, ttlSeconds});
    this.client.sessions.delete(this.id);
  }
  async destroy() {
    await unary(this.client.rpc, "destroySession", {id: this.id, resumeToken: this.value.resumeToken}, {deadline: Date.now() + 120000});
    this.client.sessions.delete(this.id);
  }
  async netem(node, interfaceName, impairment = {}) {
    const value = await unary(this.client.rpc, "applyFault", {sessionId: this.id, node, interface: interfaceName, netem: impairment});
    return new Fault(this, value);
  }
  async setLink(node, interfaceName, up) {
    const value = await unary(this.client.rpc, "applyFault", {sessionId: this.id, linkState: {node, interface: interfaceName, up}});
    return new Fault(this, value);
  }
  runTimeline(actions) { return unary(this.client.rpc, "runTimeline", {sessionId: this.id, actions}); }
}

class Node {
  constructor(session, name) { this.session = session; this.name = name; }
  ref() { return {sessionId: this.session.id, node: this.name}; }
  exec(argv, {stdin = Buffer.alloc(0), timeoutMillis = 120000} = {}) {
    return unary(this.session.client.rpc, "exec", {node: this.ref(), argv, stdin, timeoutMillis});
  }
  put(destination, content, mode = 0o600) {
    return unary(this.session.client.rpc, "put", {node: this.ref(), path: destination, content, mode});
  }
  lifecycle(action, bootstrap) {
    return unary(this.session.client.rpc, "lifecycle", {node: this.ref(), action, bootstrap}, {deadline: Date.now() + 120000});
  }
  crash() { return this.lifecycle("CRASH"); }
  powerOff() { return this.lifecycle("POWER_OFF"); }
  start() { return this.lifecycle("START"); }
  restart() { return this.lifecycle("RESTART"); }
  prepareReplacement(bootstrap) { return this.lifecycle("REPLACE", bootstrap); }
  // Deprecated: preparation only; follow with explicit native Plan/Apply.
  replace(bootstrap) { return this.prepareReplacement(bootstrap); }
}

class Fault {
  constructor(session, value) { this.session = session; this.value = value; }
  get id() { return this.value.id; }
  revert() { return unary(this.session.client.rpc, "revertFault", {sessionId: this.session.id, id: this.id}); }
}

module.exports = {Client, Session, Node, Fault};
