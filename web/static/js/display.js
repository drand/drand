const latestDiv = document.querySelector('#latest');
const historyDiv = document.querySelector('#history');
const nodesDiv = document.getElementsByClassName('map');
window.identity = "";
window.distkey = "";

//counter used to navigate through randomness indexes
var currRound = "0";
var idBar = -1;

/**
* displayRandomness is the main function which display
* the latest randomness and nodes when opening the page
**/
function displayRandomness() {
  var identity = window.identity;
  var distkey = window.distkey;

  //start the progress bar
  move();
  //print randomness
  fetchAndVerify(identity, distkey)
  .then(function (fulfilled) {
    printRound(fulfilled.randomness, fulfilled.previous, fulfilled.round, true);
  })
  .catch(function (error) {
    printRound(error.randomness, error.previous, error.round, false);
  });
  //print servers
  printNodes();
}

/**
* move handles the progress bar
**/
function move() {
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

/**
* printRound formats and prints the given randomness with interactions
**/
function printRound(randomness, previous, round, verified) {
  if (round <= currRound || round == undefined) {
    return
  }
  currRound = round;

  //print randomness as current
  var p = document.createElement("pre");
  var p2 = document.createElement("p");
  digestMessage(randomness).then(digestValue => {
    var randomness = hexString(digestValue);
    var quarter = Math.ceil(randomness.length / 4);
    var s1 = randomness.slice(0, quarter);
    var s2 = randomness.slice(quarter, 2*quarter);
    var s3 = randomness.slice(2*quarter, 3*quarter);
    var s4 = randomness.slice(3*quarter);
    var randomness_4lines =  s1 + '\n' + s2 + "\n" + s3 + "\n" + s4;
    var textnode = document.createTextNode(randomness_4lines);
    p.appendChild(textnode);
    latestDiv.replaceChild(p, latestDiv.childNodes[0]);
  });
  if (verified) {
    var textnode2 = document.createTextNode(round + " : verified");
  } else {
    var textnode2 = document.createTextNode(round + " : unverified");
  }
  p2.appendChild(textnode2);
  latestDiv.replaceChild(p2, latestDiv.childNodes[1]);

  //print JSON when clicked
  p.onmouseover = function() { p.style.textDecoration = "underline"; };
  p.onmouseout = function() {p.style.textDecoration = "none";};
  var jsonStr = '{"round":'+round+',"previous":"'+previous+ '","randomness":{"gid": 21,"point":"'+randomness+ '"}}';
  var modal = document.getElementById("myModal");
  p.onclick = function() {
    var modalcontent = document.getElementById("jsonHolder");
    if (identity.TLS){
      modalcontent.innerHTML = 'Randomness fetched from https://'+ identity.Address + '/api/public:\n <pre>' + JSON.stringify(JSON.parse(jsonStr),null,2) + "</pre>";
    } else {
      modalcontent.innerHTML = 'Randomness fetched from http://'+ identity.Address + '/api/public:\n <pre>' + JSON.stringify(JSON.parse(jsonStr),null,2) + "</pre>";
    }
    modal.style.display = "block";
  };
  window.onclick = function(event) {
    if (event.target == modal) {
      modal.style.display = "none";
    }
  }
}

/**
* printNodes prints interactive map of drand nodes
**/
function printNodes() {
  var nodes = [
    JSON.parse('{"addr": "drand.cothority.net:7003", "lat": 48.86, "lon": 2.3444}'),
    JSON.parse('{"addr": "drand.zerobyte.io:8888", "lat": 41.827637, "lon": 2.462732}'),
    JSON.parse('{"addr": "drand.nikkolasg.xyz:8888", "lat": 50.989125, "lon": 9.205674}'),
    JSON.parse('{"addr": "drand.lbarman.ch:443", "lat": 45.289125, "lon": 13.205674}'),
    JSON.parse('{"addr": "drand.kudelskisecurity.com:443", "lat": 39.789125, "lon": 9.205674}'),
    JSON.parse('{"addr": "drand.protocol.ai:8080", "lat": 44.667, "lon": -122.833}'),
    JSON.parse('{"addr": "random.uchile.cl:8080", "lat": -33.781682, "lon": -70.924195}'),
    JSON.parse('{"addr": "drand.cloudflare.com:443", "lat": 37.792032, "lon": -122.394613}')
  ];

  $(function () {
    $(".mapcontainer").mapael({
      //default settings
      map: {
        name: "world_countries",
        defaultArea: {
          attrs: {
            fill: "#d1d1d1"
            , stroke: "#d1d1d1"
          },
          attrsHover: {
            fill: "#d1d1d1"
            , stroke: "#d1d1d1"
          }
        },
        defaultPlot: {
          size:20,
          factor: 0.6,
          attrs: {
            fill:"#99D19A",
            stroke: "#fff"
          },
          eventHandlers: {
            click: function (e, id, mapElem, textElem, elemOptions) {
              window.identity = {Address: elemOptions.tooltip.content, TLS: true};
              refresh();
            }
          }
        }
      }
    });
    //for each node, look at status and show by adding to plot list if up
    for (let id in nodes) {
      var node = nodes[id];
      isUp(nodes[id].addr, true)
      .then((rand) => {
        var node = nodes[id];
        var updatedOptions = JSON.parse('{"newPlots": {"' + node.addr +'": {"latitude":"' + node.lat +'", "longitude":"' + node.lon + '", "tooltip": { "content":"' + node.addr +'"}}}}');
        $(".mapcontainer").trigger('update', updatedOptions);
      })
      .catch((err) => {console.log(nodes[id].addr + ' is down');});
    }
  });
}

/**
* isUp decides if node is reachable by trying to fetch randomness
**/
function isUp(addr, tls) {
  return new Promise(function(resolve, reject) {
    fetchPublic({Address: addr, TLS: tls})
    .then((rand) => {resolve(true);})
    .catch((error) => {reject(false);});
  });
}

/**
* goToPrev navigates to previous randomness output
**/
function goToPrev() {
  if (currRound == 0) {
    return
  }
  currRound -= 2;
  round = currRound + 1;
  //stop the 60s chrono and progress bar
  window.clearInterval(id);
  window.clearInterval(idBar);
  var elem = document.getElementById("myBar");
  elem.style.width = 0 + '%';
  //print previous rand
  var identity = window.identity;
  var distkey = window.distkey;
  fetchAndVerifyRound(identity, distkey, round)
  .then(function (fulfilled) {
    printRound(fulfilled.randomness, fulfilled.previous, round, true);
  })
  .catch(function (error) {
    printRound(error.randomness, error.previous, round, false);
  });
}

/**
* goToNext navigates to next randomness output
**/
function goToNext() {
  getLatestIndex().then((latestRound) => {
    if (currRound == latestRound) {
      //we cannot go further
      return
    }
    if (currRound + 1 == latestRound) {
      //sync with latest randomness
      refresh();
      return
    }
    //update index
    round = currRound + 1;
    //stop the 60s chrono and progress bar
    window.clearInterval(id);
    window.clearInterval(idBar);
    var elem = document.getElementById("myBar");
    elem.style.width = 0 + '%';
    //print next rand
    var identity = window.identity;
    var distkey = window.distkey;
    fetchAndVerifyRound(identity, distkey, round)
    .then(function (fulfilled) {
      printRound(fulfilled.randomness, fulfilled.previous, round, true);
    })
    .catch(function (error) {
      printRound(error.randomness, error.previous, round, false);
    });
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
**/
function getLatestIndex() {
  return new Promise(function(resolve, reject) {
    fetchPublic(window.identity).then((rand) => {resolve(rand.round);})
  });
}

/**
* digestMessage and hexString are used to hash the signature
**/
function digestMessage(message) {
  const encoder = new TextEncoder();
  const data = encoder.encode(message);
  return window.crypto.subtle.digest('SHA-512', data);
}

function hexString(buffer) {
  const byteArray = new Uint8Array(buffer);

  const hexCodes = [...byteArray].map(value => {
    const hexCode = value.toString(16);
    const paddedHexCode = hexCode.padStart(2, '0');
    return paddedHexCode;
  });

  return hexCodes.join('');
}
