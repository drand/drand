const fetchCommand = "fetchVerify";
const loadDrand = "loadDrand";

async function fetchRand(data) {
    var res = {}
    try {
        console.log("webworker.js: received fetch command",JSON.stringify(data));
        const rand = await drandjs.fetchAndVerify(data.identity, 
                                    data.distkey, 
                                    data.round)
        self.postMessage({
            cmd: fetchCommand,
            randomness: rand,
        });

    } catch (e) {
        if (e instanceof drandjs.InvalidVerification) {
            self.postMessage({
                cmd:fetchCommand,
                error: "Invalid verification",
                invalid: e.rand,
                request: data,
            });
        } else {
            self.postMessage({
                cmd:fetchCommand,
                error: e.message,
                request: data,
            });
        }
    }
}
self.onmessage = function(message) {
  var data = message.data;
  switch (data.cmd) {
    case loadDrand:
        importScripts(data.path);
        console.log("webworker.js imported drandjs.");
        break;
    case fetchCommand:
        fetchRand(data).then("webworker.js: finished fetching");
        break;
    default:
      console.log("worker: unknown command ",data.cmd);
  };
};
