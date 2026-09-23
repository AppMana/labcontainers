export interface ClientOptions { socket?: string; stateDir?: string; labd?: string }
export interface StartOptions { ttlSeconds?: number; labels?: Record<string, string> }
export interface ExecOptions { stdin?: Buffer; timeoutMillis?: number }
export interface ExecResult { stdout: Buffer; stderr: Buffer; exitCode: number }

export class Client {
  readonly socket: string;
  readonly stateDirectory?: string;
  static dial(socket: string): Promise<Client>;
  static launch(options?: ClientOptions): Promise<Client>;
  start(spec: object, options?: StartOptions): Promise<Session>;
  resume(id: string): Promise<Session>;
  close(): Promise<void>;
}
export class Session {
  readonly id: string;
  node(name: string): Node;
  keep(ttlSeconds?: number): Promise<void>;
  destroy(): Promise<void>;
  netem(node: string, interfaceName: string, impairment?: object): Promise<Fault>;
  setLink(node: string, interfaceName: string, up: boolean): Promise<Fault>;
  runTimeline(actions: object[]): Promise<object>;
}
export class Node {
  exec(argv: string[], options?: ExecOptions): Promise<ExecResult>;
  put(destination: string, content: Buffer, mode?: number): Promise<void>;
  powerOff(): Promise<void>;
  start(): Promise<void>;
  restart(): Promise<void>;
  prepareReplacement(bootstrap?: object): Promise<void>;
  /** @deprecated Preparation only; requires subsequent explicit Plan/Apply. */
  replace(bootstrap?: object): Promise<void>;
}
export class Fault { readonly id: string; revert(): Promise<void> }
