const latestDiv = document.querySelector('#latest');
const roundDiv = document.querySelector('#round');
const verifyButton = document.querySelector('#verify');
const nodesListDiv = document.querySelector('#nodes');
var locationMap = new Map();

window.verified = false;

//interval id for bar progress
var idBar = -1;

const fetchCommand = "fetchVerify";
const randKey = "randomness";
window.worker.addEventListener('message', function(e) {
    var data = e.data;
    switch (data.cmd) {
        case fetchCommand:
            console.log("display.js: received fetch results:",data);
            if ("randomness" in data) {
                const d = data.randomness;
                window.verified = d.verified;
                window.distkey = d.distkey;
                const randomness = drandjs.toHexString(drandjs.sha256(d.signature));
                printRound(randomness, d.previous, d.round, d.signature);
                setVerified(true);
            } else if ("error" in data) {
                setRound(data.request.round);
                if (data.error.includes("verification")) { 
                    const d = data.invalid;
                    const randomness = drandjs.toHexString(drandjs.sha256(d.signature));
                    setRandomness(sliceRandomness(d.randomness));
                    console.log("unable to verify with current hash-to-curve method");
                    // XXX: currently the library use the Boneh hash to curve
                    // method which isn't implemented in JS - Need to do a
                    // webassembly wrapper around the rust library
                    printRound(randomness, d.previous, d.round, d.signature);
                    setVerified(false, "Randomness fetched correctly");
                    //setVerified(false, "Invalid verification");
                } else {
                    setVerified(false, " Error during verification");
                    var p = document.createElement("pre");
                    var textnode = document.createTextNode(data.error);
                    p.appendChild(textnode);
                    latestDiv.replaceChild(p, latestDiv.childNodes[0]);
                    console.log("display.js ERROR : " + data.error);
                    // showError();
                }
            } else {
                throw new Error("THAT SHOULD NOT HAPPENS");
            }
    }
}, false);

/**
* displayRandomness is the main function which display
* the latest randomness and nodes when opening the page
* the first contacted node is picked at random from group file
**/
async function displayRandomness() {
  window.identity = window.identity || await findFirstNode();
  startProgressBar();
  //print randomness and update verfified status
  console.log("display.js fecthing randomness - from ", window.identity);
  requestFetch(window.identity, window.distkey);
  printNodesList(window.identity);
}

function requestFetch(id, dist, round) {
    setButtonInProgress();
    if (round == null) {
        setRound("fetching latest round...");
    } else {
        setRound(round);
    }
    setRandomness("...\n...");
    window.worker.postMessage({
      cmd: fetchCommand, 
      identity: id,
      distkey: dist,
      round: round, 
    });
}

/**
* startProgressBar handles the progress bar
**/
function startProgressBar() {
  var elem = document.getElementById("myBar");
  var width = 0;
  if (idBar != -1) {
    window.clearInterval(idBar);
  }
  idBar = setInterval(frame, 60);
  function frame() {
    if (width >= 100) {
      clearInterval(idBar);
    } else {
      width += 0.1;
      elem.style.width = width + '%';
    }
  }
}

function setRandomness(str) {
  var p = document.createElement("pre");
    var textnode = document.createTextNode(str);
  p.appendChild(textnode);
  latestDiv.replaceChild(p, latestDiv.childNodes[0]);
  return p;
}

function sliceRandomness(randomness) {
  var quarter = Math.ceil(randomness.length/2);
  var s1 = randomness.slice(0, quarter);
  var s2 = randomness.slice(quarter, 2*quarter);
  var randomness_4lines =  s1 + '\n' + s2;
  return randomness_4lines;
}

/**
* printRound formats and prints the given randomness with interactions
**/
function printRound(randomness, previous, round, signature) {
  //print randomness as current
  var r4l = sliceRandomness(randomness);
  var p = setRandomness(r4l);
  //print JSON when clicked
  p.onmouseover = function() { p.style.textDecoration = "underline"; p.style.cursor = "pointer"};
  p.onmouseout = function() {p.style.textDecoration = "none";};
  var jsonStr = '{"round":'+round+',"previous":"'+previous+'","signature":"'+signature+'","randomness":"'+randomness+'"}';
  var modal = document.getElementById("myModal");
  p.onclick = function() {
    if (window.identity.TLS){
      var reqURL = 'https://'+ window.identity.Address + '/api/public';
    } else {
      var reqURL = 'http://'+ window.identity.Address + '/api/public';
    }
    var modalcontent = document.getElementById("jsonHolder");
    modalcontent.innerHTML = 'Request URL: <strong>'+ reqURL + '</strong> <br> Raw JSON: <pre>' + JSON.stringify(JSON.parse(jsonStr),null,2) + "</pre>";
    modal.style.display = "block";
  };
  window.onclick = function(event) {
    if (event.target == modal) {
      modal.style.display = "none";
    }
  }

  //index info
  setRound(round);
}

/**
* printNodeList prints interactive list of drand nodes
**/
async function printNodesList(identity) {
    var group = undefined;
    try {
        group = await drandjs.fetchGroup(identity);
    } catch (e) {
        console.error("printNodesList coult not fetch group from ",identity,":",e);
        return;
    }
    nodesListDiv.innerHTML="";
    for(var i = 0; i < group.nodes.length; i++) {
        let addr = group.nodes[i].address;
        let host = addr.split(":")[0];
        let port = addr.split(":")[1];
        // when not present, assume not TLS
        // gRPC or golang/json doesn't put TLS field when false...
        let tls = group.nodes[i].TLS || false;

        let line = document.createElement("tr");
        let statusCol = document.createElement("td");
        // run them in parallel
        isUp(addr, tls).then((rand) => {
            statusCol.innerHTML = '<td> &nbsp;&nbsp;&nbsp; ✔️ </td>';
            statusCol.style.color= "transparent";
            statusCol.style.textShadow= "0 0 0 green";
            console.log(addr," is  up");
        }).catch(() => {
            statusCol.innerHTML = '<td> &nbsp;&nbsp;&nbsp; 🚫 </td>';
        });
    
        line.appendChild(statusCol);

        let addrCol = document.createElement("td");
        addrCol.innerHTML = '<td>' + host + '</td>';
        addrCol.onmouseover = function() { addrCol.style.textDecoration = "underline"; };
        addrCol.onmouseout = function() {addrCol.style.textDecoration = "none";};
        addrCol.onclick = function() {
          window.identity = {Address: addr, TLS: tls};
          refresh();
        };
        line.appendChild(addrCol);

        let portCol = document.createElement("td");
        portCol.innerHTML = '<td>' +port+'</td>';
        line.appendChild(portCol);

        let tlsCol = document.createElement("td");
        tlsCol.innerHTML = '<td> non tls </td>';
        if (tls) {
          tlsCol.innerHTML = '<td> tls </td>';
        }
        line.appendChild(tlsCol);

        var loc = locationMap.get(host);
        if (loc == undefined) { //did not fill map loc yet
          function handleResponse(json) {
            locationMap.set(host, json.country_code2);
            refresh();
          }
          getLoc(host, handleResponse);
        }
        loc = locationMap.get(host);
        if (loc == undefined) {
          loc = " ";
        }
        let countryCol = document.createElement("td");
        countryCol.innerHTML = '<td>' + loc + '</td>';
        line.appendChild(countryCol);

        let linkCol = document.createElement("td");
        linkCol.innerHTML = '<td><a title="https://' + addr + '/api/public" href="https://' + addr + '/api/public"><i class="fas fa-external-link-alt"></i></a></td>';
        linkCol.style.textAlign="center";
        line.appendChild(linkCol);

        if (addr == window.identity.Address) {
          line.style.fontWeight="bold";
        }
        nodesListDiv.appendChild(line);
    }
}

/**
* isUp decides if node is reachable by trying to fetch randomness
**/
async function isUp(addr, tls) {
    try {
        await drandjs.fetchLatest({Address: addr, TLS: tls})
        console.log(addr," is up !");
        return true;
    } catch (e) {
        console.log(addr, ": error reaching out: ",e);
        throw e;
    }
}

/**
* goToPrev navigates to previous randomness output
**/
function goToPrev() {
  var round = getRound() - 1;
  //stop the 60s chrono and progress bar
  window.clearInterval(id);
  window.clearInterval(idBar);
  var elem = document.getElementById("myBar");
  elem.style.width = 0 + '%';
  //print previous rand
  
  console.log("display.js fetching previous randomness");
  requestFetch(window.identity, window.distkey,round);
}

/**
* goToNext navigates to next randomness output
**/
function goToNext() {
  getLatestIndex().then((latestRound) => {
    if (getRound() == latestRound) {
      console.log("display.js: goToNext returns since already at latest round");
      //we cannot go further
      return
    }
    //update index
    var round = getRound() + 1;
    //stop the 60s chrono and progress bar
    window.clearInterval(id);
    window.clearInterval(idBar);
    var elem = document.getElementById("myBar");
    elem.style.width = 0 + '%';
    //print next rand
    console.log("display.js sending command to fetch next round",round);
    requestFetch( window.identity, window.distkey,round);
  });
}

/**
* refresh goes back to latest output
**/
function refresh() {
  window.clearInterval(id);
  displayRandomness();
  window.setInterval(displayRandomness, 60000);
}

/**
* getLatestIndex returns the index of the latest randomness
* used to get upper bound for the prev/next navigation
**/
function getLatestIndex() {
  return new Promise(function(resolve, reject) {
    drandjs.fetchLatest(window.identity).then((rand) => {resolve(rand.round);})
  });
}

/**
* setVerified sets the msg for the button and sets the icon depending on the
* correct argument
**/
function setVerified(correct, msg) {
  if (correct) {
    verifyButton.innerHTML = '<a class="button alt icon solid small"> <i class="fas fa-check"></i> &nbsp; randomness verified </a>';
  } else {
    verifyButton.innerHTML = '<a class="button alt icon solid small"> <i class="fas fa-times"></i> &nbsp; ' + msg + ' </a>';
  }
}

function setButtonInProgress() {
    verifyButton.innerHTML = '<a class="button alt icon solid small"> <i class="fas fa-cog"></i> &nbsp; Randomness being fetched and verified ... </a>';
}

function setRound(round) {
  var p2 = document.createElement("pre");
  var textnode2 = document.createTextNode(round);
  p2.appendChild(textnode2);
  roundDiv.replaceChild(p2, roundDiv.childNodes[0]);
}

function getRound() {
  return parseInt(roundDiv.childNodes[0].innerText);
}

function shuffle(array) {
  for (let i = array.length - 1; i > 0; i--) {
    let j = Math.floor(Math.random() * (i + 1)); // random index from 0 to i
    [array[i], array[j]] = [array[j], array[i]];
  }
}
/**
* findFirstNode picks a first node to contact at random from the up servers
* starts by reading last configuration file from github repo, filters the
* addresses and tries until success to contact a server with tls from the pool
**/
async function findFirstNode() {
  try {
    const resp = await fetch('https://raw.githubusercontent.com/dedis/drand/master/deploy/latest/group.toml')
    const text = await resp.text();
    const addrList = Object.values(text.split('\n')).filter(str => str.includes("Address")).map(item => item.substring(13, item.length - 1));
    const shuffled = shuffle(addrList);
    for (var i =0; i < shuffled.length; i++) {
        const addr = addrList[shuffled];
        try {
            // try by default TLS nodes if we haven't got any
            await isUp(addr,true);
            console.log(addr, "is UP!");
            return addr;
        } catch (e) {
            console.log(e);
        }
    }
  } catch (e) {
    alert(`could not get the group from github, reload the page:\n ${e}`);
  }
}

/**
* sleep makes the main thread wait ms milliseconds before continuing
**/
function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/**
* getLoc communicates with dns-js.com and geoIP.com APIs
**/
function getLoc(domain, callback) {
  return;
  var xhr = new XMLHttpRequest();
  URL = "https://www.dns-js.com/api.aspx";
  xhr.open("POST", URL);
  xhr.onreadystatechange = function () {
    if (this.readyState === XMLHttpRequest.DONE && this.status === 200) {
      let data = JSON.parse(xhr.response);
      var ip = data[0].Address;
      setIPAddressParameter(ip);
      setExcludesParameter("ip");
      setFieldsParameter("country_code2");
      getGeolocation(callback, "ca50c203abfa45a39fe376f3ba9d0a3f");
    }
  }
  xhr.send(JSON.stringify({Action: "Query", Domain: domain,Type: 1}));
}
