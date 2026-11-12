import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";
import ts from "typescript";

// Exercise the actual view predicate with captured idle and active peer states.
const source = readFileSync(new URL("../src/views/Devices.vue", import.meta.url), "utf8");
const predicate = source.slice(source.indexOf("const getPeerLinkBadge ="), source.indexOf("const canConnectRDP ="));
const js = ts.transpile(predicate);
function badge(state) {
  return runInNewContext(`${js}\ngetPeerLinkBadge('peer')`, {
    peerLinks: { value: { peer: state } },
  });
}
assert.equal(badge(undefined), null);
assert.equal(badge({ online: true, active: false, relay: "private", is_direct: false })?.label, "链路待建立");
assert.equal(badge({ online: false, active: false, relay: "private", is_direct: true }), null);
assert.equal(badge({ online: true, active: false, relay: "private", is_direct: true })?.label, "链路待建立");
assert.equal(badge({ online: true, active: true, relay: "private", is_direct: true })?.label, "P2P 直连");
assert.equal(badge({ online: true, active: true, relay: "private", is_direct: false })?.label, "DERP 中继 (private)");
assert.equal(badge({ online: true, active: true, relay: "", is_direct: false })?.label, "链路待确认");
console.log("peer link badge: 7 cases passed");

const loaderSource = source.slice(source.indexOf("const loadPeerLinks ="), source.indexOf("const loadLocalStatus ="));
const pending = [];
const peerLinks = { value: { peer: { is_direct: true } } };
const load = runInNewContext(ts.transpile(`let latestPeerLinkRequest = 0;\n${loaderSource}\nloadPeerLinks;`, { target: ts.ScriptTarget.ES2020 }), {
  peerLinks,
  GetPeerLinkStates: () => new Promise((resolve, reject) => pending.push({ resolve, reject })),
});
const failed = load();
pending.shift().reject(new Error("bridge unavailable"));
await failed;
assert.equal(Object.keys(peerLinks.value).length, 0);
const older = load();
const newer = load();
pending[1].resolve({ peer: { is_direct: false } });
await newer;
pending[0].resolve({ peer: { is_direct: true } });
await older;
assert.equal(peerLinks.value.peer.is_direct, false);
console.log("peer link refresh: failed query and out-of-order response cases passed");
