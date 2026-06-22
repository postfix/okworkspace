import "@testing-library/jest-dom";

// jsdom does not implement EventSource, but components that open SSE streams
// (PresenceIndicator's subscribePresence, the attachment extraction-status chip)
// construct one on mount. Provide a minimal inert stub so mounting those
// components under test does not throw `ReferenceError: EventSource is not
// defined`. The stub never emits — tests that assert on SSE behaviour drive the
// component through the api/client seam (mocked per-test), not through real
// network events; this only needs to be constructible and closable.
if (typeof globalThis.EventSource === "undefined") {
  class MockEventSource {
    static readonly CONNECTING = 0;
    static readonly OPEN = 1;
    static readonly CLOSED = 2;
    readonly CONNECTING = 0;
    readonly OPEN = 1;
    readonly CLOSED = 2;
    url: string;
    readyState = 0;
    withCredentials = false;
    onopen: ((this: EventSource, ev: Event) => unknown) | null = null;
    onmessage: ((this: EventSource, ev: MessageEvent) => unknown) | null = null;
    onerror: ((this: EventSource, ev: Event) => unknown) | null = null;
    constructor(url: string | URL) {
      this.url = String(url);
    }
    addEventListener() {}
    removeEventListener() {}
    dispatchEvent() {
      return false;
    }
    close() {
      this.readyState = 2;
    }
  }
  // Assign the stub onto the global; the cast keeps TS happy without pulling the
  // full lib.dom EventSource surface.
  globalThis.EventSource = MockEventSource as unknown as typeof EventSource;
}
